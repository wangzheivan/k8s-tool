package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	logStatusPending           = "pending"
	logStatusRunning           = "running"
	logStatusOK                = "ok"
	logStatusAgentConnectError = "agent-connect-error"
	logStatusCollectorError    = "collector-error"
	logStatusDownloadError     = "download-error"
	logStatusTimeout           = "timeout"
)

const (
	defaultLogCollectionRoot = "/tmp/k8s-tool-log-collections"
	defaultCollectorScript   = "/usr/local/share/k8s-tool/rancher2_logs_collector.sh"
)

var safeLogIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type LogCollectRequest struct {
	Days int `json:"days"`
}

type LogNodeResult struct {
	NodeName     string `json:"nodeName"`
	NodeIP       string `json:"nodeIP"`
	AgentPod     string `json:"agentPod,omitempty"`
	AgentPodIP   string `json:"agentPodIP,omitempty"`
	AgentURL     string `json:"agentURL,omitempty"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	StartedAt    string `json:"startedAt,omitempty"`
	CompletedAt  string `json:"completedAt,omitempty"`
	DurationMS   int64  `json:"durationMS"`
	ArtifactID   string `json:"artifactID,omitempty"`
	ArtifactName string `json:"artifactName,omitempty"`
	SizeBytes    int64  `json:"sizeBytes"`
}

type LogCollectionSummary struct {
	CollectionID       string          `json:"collectionID,omitempty"`
	Running            bool            `json:"running"`
	StartedAt          string          `json:"startedAt,omitempty"`
	CompletedAt        string          `json:"completedAt,omitempty"`
	Days               int             `json:"days,omitempty"`
	NodeCount          int             `json:"nodeCount"`
	CompletedNodeCount int             `json:"completedNodeCount"`
	FailedNodeCount    int             `json:"failedNodeCount"`
	TotalBytes         int64           `json:"totalBytes"`
	DownloadReady      bool            `json:"downloadReady"`
	DownloadURL        string          `json:"downloadURL,omitempty"`
	Error              string          `json:"error,omitempty"`
	Results            []LogNodeResult `json:"results"`
}

func (s *server) handleLogsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeLogsStatus(w)
}

func (s *server) handleLogsCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LogCollectRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if !validLogDays(req.Days) {
		http.Error(w, "days must be one of 1, 3, 7, 14", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.logs.Running {
		summary := cloneLogSummary(s.logs)
		s.mu.Unlock()
		writeJSON(w, summary)
		return
	}
	collectionID := newLogCollectionID()
	startedAt := time.Now().Format(time.RFC3339)
	s.logs = LogCollectionSummary{
		CollectionID: collectionID,
		Running:      true,
		StartedAt:    startedAt,
		Days:         req.Days,
		Results:      []LogNodeResult{},
	}
	summary := cloneLogSummary(s.logs)
	s.mu.Unlock()
	writeJSON(w, summary)

	go s.runLogCollection(context.Background(), collectionID, req.Days, startedAt)
}

func (s *server) handleLogsDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	collectionID := strings.TrimPrefix(r.URL.Path, "/api/logs/download/")
	if !safeLogID(collectionID) {
		http.Error(w, "invalid collection id", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	summary := cloneLogSummary(s.logs)
	s.mu.RUnlock()
	if summary.CollectionID != collectionID || !summary.DownloadReady {
		http.NotFound(w, r)
		return
	}
	path := serverLogArchivePath(collectionID)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(path)))
	http.ServeFile(w, r, path)
}

func (s *server) runLogCollection(parent context.Context, collectionID string, days int, startedAt string) {
	log.Printf("logs collection started collection=%s days=%d", collectionID, days)
	s.cleanupOldLogCollections()
	ctx, cancel := context.WithTimeout(parent, s.logCollectTimeout)
	defer cancel()

	nodes, err := s.listNodes(ctx)
	if err != nil {
		s.finishLogCollection(LogCollectionSummary{
			CollectionID: collectionID,
			Running:      false,
			StartedAt:    startedAt,
			CompletedAt:  time.Now().Format(time.RFC3339),
			Days:         days,
			Error:        err.Error(),
			Results:      []LogNodeResult{},
		})
		return
	}
	pods, err := s.listAgentPods(ctx)
	if err != nil {
		s.finishLogCollection(LogCollectionSummary{
			CollectionID: collectionID,
			Running:      false,
			StartedAt:    startedAt,
			CompletedAt:  time.Now().Format(time.RFC3339),
			Days:         days,
			NodeCount:    len(nodes),
			Error:        err.Error(),
			Results:      []LogNodeResult{},
		})
		return
	}

	agentsByNode := agentPodsByNode(pods)
	results := make([]LogNodeResult, 0, len(nodes))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, s.logCollectParallel)

	for _, n := range nodes {
		n := n
		agent, ok := agentsByNode[n.Metadata.Name]
		if !ok {
			results = append(results, logNodeResult(n, logStatusAgentConnectError, "no k8s-tool-agent pod is running on this node"))
			continue
		}
		if agent.Status.Phase != "Running" || agent.Status.PodIP == "" {
			result := logNodeResult(n, logStatusAgentConnectError, fmt.Sprintf("agent pod %s is not checkable: phase=%s podIP=%q", agent.Metadata.Name, agent.Status.Phase, agent.Status.PodIP))
			result.AgentPod = agent.Metadata.Name
			result.AgentPodIP = agent.Status.PodIP
			results = append(results, result)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, err := s.collectAgentLogs(ctx, n, agent, collectionID, days)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed := logNodeResult(n, logStatusAgentConnectError, err.Error())
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					failed.Status = logStatusTimeout
				}
				failed.AgentPod = agent.Metadata.Name
				failed.AgentPodIP = agent.Status.PodIP
				failed.AgentURL = fmt.Sprintf("http://%s:%d/api/logs/collect", agent.Status.PodIP, s.agentPort)
				results = append(results, failed)
				return
			}
			results = append(results, result)
		}()
	}
	wg.Wait()

	summary := LogCollectionSummary{
		CollectionID: collectionID,
		Running:      false,
		StartedAt:    startedAt,
		CompletedAt:  time.Now().Format(time.RFC3339),
		Days:         days,
		NodeCount:    len(nodes),
		Results:      results,
	}
	finalizeLogSummary(&summary)
	if err := s.buildServerLogArchive(collectionID, &summary); err != nil {
		summary.Error = err.Error()
		summary.DownloadReady = false
		if summary.Results == nil {
			summary.Results = []LogNodeResult{}
		}
	}
	s.finishLogCollection(summary)
	log.Printf("logs collection completed collection=%s nodes=%d ok=%d failed=%d bytes=%d ready=%t", collectionID, summary.NodeCount, summary.CompletedNodeCount, summary.FailedNodeCount, summary.TotalBytes, summary.DownloadReady)
}

func (s *server) collectAgentLogs(ctx context.Context, n node, agent pod, collectionID string, days int) (LogNodeResult, error) {
	endpoint := fmt.Sprintf("http://%s:%d/api/logs/collect", agent.Status.PodIP, s.agentPort)
	body, _ := json.Marshal(LogCollectRequest{Days: days})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return LogNodeResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: s.logCollectTimeout + 5*time.Second}).Do(req)
	if err != nil {
		return LogNodeResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return LogNodeResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LogNodeResult{}, fmt.Errorf("agent returned %s: %s", resp.Status, string(raw))
	}
	var result LogNodeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return LogNodeResult{}, err
	}
	if result.NodeName == "" {
		result.NodeName = n.Metadata.Name
	}
	if result.NodeIP == "" {
		result.NodeIP = nodeInternalIP(n)
	}
	result.AgentPod = agent.Metadata.Name
	result.AgentPodIP = agent.Status.PodIP
	result.AgentURL = endpoint
	if result.Status != logStatusOK {
		return result, nil
	}
	if err := s.downloadAgentLogArtifact(ctx, agent, collectionID, result); err != nil {
		result.Status = logStatusDownloadError
		result.Message = err.Error()
		return result, nil
	}
	return result, nil
}

func (s *server) downloadAgentLogArtifact(ctx context.Context, agent pod, collectionID string, result LogNodeResult) error {
	if !safeLogID(result.ArtifactID) {
		return fmt.Errorf("agent returned invalid artifact id %q", result.ArtifactID)
	}
	nodeDir := filepath.Join(serverLogCollectionDir(collectionID), "nodes", safeFileName(result.NodeName))
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		return err
	}
	endpoint := fmt.Sprintf("http://%s:%d/api/logs/download/%s", agent.Status.PodIP, s.agentPort, result.ArtifactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: s.logCollectTimeout + 5*time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent artifact download returned %s: %s", resp.Status, string(raw))
	}
	name := result.ArtifactName
	if name == "" {
		name = result.ArtifactID + ".tar.gz"
	}
	name = safeFileName(name)
	out, err := os.Create(filepath.Join(nodeDir, name))
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func (s *server) buildServerLogArchive(collectionID string, summary *LogCollectionSummary) error {
	dir := serverLogCollectionDir(collectionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	finalizeLogSummary(summary)
	summary.DownloadURL = fmt.Sprintf("/api/logs/download/%s", collectionID)
	summary.DownloadReady = summary.CompletedNodeCount > 0
	if !summary.DownloadReady {
		return nil
	}
	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), raw, 0o644); err != nil {
		return err
	}
	archivePath := serverLogArchivePath(collectionID)
	if err := createTarGz(archivePath, dir); err != nil {
		return err
	}
	info, err := os.Stat(archivePath)
	if err == nil {
		summary.TotalBytes = info.Size()
	}
	return nil
}

func (s *server) finishLogCollection(summary LogCollectionSummary) {
	finalizeLogSummary(&summary)
	s.mu.Lock()
	s.logs = summary
	s.mu.Unlock()
}

func (s *server) writeLogsStatus(w http.ResponseWriter) {
	s.mu.RLock()
	summary := cloneLogSummary(s.logs)
	s.mu.RUnlock()
	if summary.Results == nil {
		summary.Results = []LogNodeResult{}
	}
	writeJSON(w, summary)
}

func (s *server) cleanupOldLogCollections() {
	root := logCollectionRoot()
	if s.logRetentionHours <= 0 {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-s.logRetentionHours)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}

func handleAgentLogsCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LogCollectRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if !validLogDays(req.Days) {
		http.Error(w, "days must be one of 1, 3, 7, 14", http.StatusBadRequest)
		return
	}
	timeout := time.Duration(envInt("LOG_COLLECTION_TIMEOUT_SECONDS", 900)) * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	result := runAgentLogCollector(ctx, collectAgentInfo(), req.Days)
	writeJSON(w, result)
}

func handleAgentLogsDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	artifactID := strings.TrimPrefix(r.URL.Path, "/api/logs/download/")
	if !safeLogID(artifactID) {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	path, err := agentArtifactPath(artifactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(path)))
	http.ServeFile(w, r, path)
}

func runAgentLogCollector(ctx context.Context, info AgentInfo, days int) LogNodeResult {
	start := time.Now()
	artifactID := newLogCollectionID()
	result := LogNodeResult{
		NodeName:   info.NodeName,
		NodeIP:     info.NodeIP,
		AgentPod:   info.PodName,
		AgentPodIP: info.PodIP,
		Status:     logStatusRunning,
		StartedAt:  start.Format(time.RFC3339),
		ArtifactID: artifactID,
	}
	if !validLogDays(days) {
		result.Status = logStatusCollectorError
		result.Message = "days must be one of 1, 3, 7, 14"
		result.CompletedAt = time.Now().Format(time.RFC3339)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	script := env("LOG_COLLECTOR_SCRIPT", defaultCollectorScript)
	if _, err := os.Stat(script); err != nil {
		result.Status = logStatusCollectorError
		result.Message = err.Error()
		result.CompletedAt = time.Now().Format(time.RFC3339)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	artifactDir := filepath.Join(logCollectionRoot(), artifactID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		result.Status = logStatusCollectorError
		result.Message = err.Error()
		result.CompletedAt = time.Now().Format(time.RFC3339)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	args := []string{script, "-d", artifactDir, "-s", strconv.Itoa(days), "-f"}
	if os.Getenv("LOG_COLLECTOR_FORCE_POD_DISTRO") == "true" {
		args = append(args, "-r", "pod")
	} else if dir := env("RKE2_DATA_DIR", "/var/lib/rancher/rke2"); dir != "" && logDirExists(dir) {
		args = append(args, "-r", "rke2", "-c", dir)
	} else if dir := env("K3S_DATA_DIR", "/var/lib/rancher/k3s"); dir != "" && logDirExists(dir) {
		args = append(args, "-r", "k3s")
	}
	cmd := exec.CommandContext(ctx, "bash", args...)
	cmd.Env = append(os.Environ(), "LANG=C")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		result.Status = logStatusTimeout
		result.Message = ctx.Err().Error()
	} else if err != nil {
		result.Status = logStatusCollectorError
		result.Message = truncateLogMessage(strings.TrimSpace(string(output)))
		if result.Message == "" {
			result.Message = err.Error()
		}
	} else {
		path, findErr := agentArtifactPath(artifactID)
		if findErr != nil {
			result.Status = logStatusCollectorError
			result.Message = findErr.Error()
		} else {
			info, statErr := os.Stat(path)
			if statErr != nil {
				result.Status = logStatusCollectorError
				result.Message = statErr.Error()
			} else {
				result.Status = logStatusOK
				result.ArtifactName = filepath.Base(path)
				result.SizeBytes = info.Size()
			}
		}
	}
	result.CompletedAt = time.Now().Format(time.RFC3339)
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func truncateLogMessage(value string) string {
	const max = 4000
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n... truncated ..."
}

func validLogDays(days int) bool {
	return days == 1 || days == 3 || days == 7 || days == 14
}

func finalizeLogSummary(summary *LogCollectionSummary) {
	if summary.Results == nil {
		summary.Results = []LogNodeResult{}
	}
	sort.Slice(summary.Results, func(i, j int) bool {
		return summary.Results[i].NodeName < summary.Results[j].NodeName
	})
	summary.CompletedNodeCount = 0
	summary.FailedNodeCount = 0
	var total int64
	errors := make([]string, 0)
	for _, result := range summary.Results {
		if result.Status == logStatusOK {
			summary.CompletedNodeCount++
			total += result.SizeBytes
		} else if result.Status != logStatusPending && result.Status != logStatusRunning {
			summary.FailedNodeCount++
			if result.Message != "" {
				errors = append(errors, fmt.Sprintf("%s: %s", result.NodeName, result.Message))
			} else {
				errors = append(errors, fmt.Sprintf("%s: %s", result.NodeName, result.Status))
			}
		}
	}
	if summary.TotalBytes == 0 {
		summary.TotalBytes = total
	}
	if summary.Error == "" && len(errors) > 0 {
		summary.Error = strings.Join(errors, "; ")
	}
	if summary.CollectionID != "" && summary.DownloadReady && summary.DownloadURL == "" {
		summary.DownloadURL = fmt.Sprintf("/api/logs/download/%s", summary.CollectionID)
	}
}

func logNodeResult(n node, status, message string) LogNodeResult {
	now := time.Now().Format(time.RFC3339)
	return LogNodeResult{
		NodeName:    n.Metadata.Name,
		NodeIP:      nodeInternalIP(n),
		Status:      status,
		Message:     message,
		StartedAt:   now,
		CompletedAt: now,
	}
}

func cloneLogSummary(summary LogCollectionSummary) LogCollectionSummary {
	out := summary
	if summary.Results != nil {
		out.Results = append([]LogNodeResult(nil), summary.Results...)
	} else {
		out.Results = []LogNodeResult{}
	}
	return out
}

func newLogCollectionID() string {
	return time.Now().UTC().Format("20060102T150405.000000000Z")
}

func logCollectionRoot() string {
	return env("LOG_COLLECTION_DIR", defaultLogCollectionRoot)
}

func serverLogCollectionDir(collectionID string) string {
	return filepath.Join(logCollectionRoot(), collectionID)
}

func serverLogArchivePath(collectionID string) string {
	return filepath.Join(logCollectionRoot(), collectionID+".tar.gz")
}

func agentArtifactPath(artifactID string) (string, error) {
	if !safeLogID(artifactID) {
		return "", fmt.Errorf("invalid artifact id")
	}
	dir := filepath.Join(logCollectionRoot(), artifactID)
	matches, err := filepath.Glob(filepath.Join(dir, "*.tar.gz"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("artifact not found")
	}
	sort.Strings(matches)
	return matches[0], nil
}

func safeLogID(value string) bool {
	return value != "" && !strings.Contains(value, "..") && safeLogIDPattern.MatchString(value)
}

func safeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func logDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func createTarGz(dest string, sourceDir string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	return filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == sourceDir {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}

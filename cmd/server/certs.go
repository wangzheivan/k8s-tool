package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	certStatusOK                = "ok"
	certStatusExpired           = "expired"
	certStatusExpiringSoon      = "expiring-soon"
	certStatusParseError        = "parse-error"
	certStatusMissingRKE2Path   = "missing-rke2-path"
	certStatusAgentConnectError = "agent-connect-error"
)

type CertificateInfo struct {
	NodeName     string `json:"nodeName"`
	NodeIP       string `json:"nodeIP"`
	Category     string `json:"category"`
	Path         string `json:"path"`
	Subject      string `json:"subject"`
	Issuer       string `json:"issuer"`
	NotBefore    string `json:"notBefore"`
	NotAfter     string `json:"notAfter"`
	DaysLeft     int    `json:"daysLeft"`
	Expired      bool   `json:"expired"`
	ExpiringSoon bool   `json:"expiringSoon"`
	ParseError   string `json:"parseError,omitempty"`
	Status       string `json:"status"`
}

type CertNodeResult struct {
	NodeName          string            `json:"nodeName"`
	NodeIP            string            `json:"nodeIP"`
	Role              string            `json:"role"`
	AgentPod          string            `json:"agentPod,omitempty"`
	AgentPodIP        string            `json:"agentPodIP,omitempty"`
	AgentURL          string            `json:"agentURL,omitempty"`
	Status            string            `json:"status"`
	Message           string            `json:"message,omitempty"`
	CheckedAt         string            `json:"checkedAt"`
	DurationMS        int64             `json:"durationMS"`
	ServerCertCount   int               `json:"serverCertCount"`
	AgentCertCount    int               `json:"agentCertCount"`
	ExpiredCount      int               `json:"expiredCount"`
	ExpiringSoonCount int               `json:"expiringSoonCount"`
	ParseErrorCount   int               `json:"parseErrorCount"`
	Certificates      []CertificateInfo `json:"certificates"`
}

type CertStatusSummary struct {
	Running           bool             `json:"running"`
	StartedAt         string           `json:"startedAt,omitempty"`
	CompletedAt       string           `json:"completedAt,omitempty"`
	Error             string           `json:"error,omitempty"`
	NodeCount         int              `json:"nodeCount"`
	ServerNodeCount   int              `json:"serverNodeCount"`
	WorkerNodeCount   int              `json:"workerNodeCount"`
	CheckedNodeCount  int              `json:"checkedNodeCount"`
	TotalCertCount    int              `json:"totalCertCount"`
	ExpiredCount      int              `json:"expiredCount"`
	ExpiringSoonCount int              `json:"expiringSoonCount"`
	ParseErrorCount   int              `json:"parseErrorCount"`
	Results           []CertNodeResult `json:"results"`
	SourceErrors      []string         `json:"sourceErrors,omitempty"`
}

func (s *server) runCertStatusCheck(ctx context.Context) CertStatusSummary {
	startedAt := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	s.certs.Running = true
	s.certs.StartedAt = startedAt
	s.certs.CompletedAt = ""
	s.certs.Error = ""
	s.mu.Unlock()

	nodes, err := s.listNodes(ctx)
	if err != nil {
		summary := CertStatusSummary{Running: false, StartedAt: startedAt, CompletedAt: time.Now().Format(time.RFC3339), Error: err.Error(), Results: []CertNodeResult{}}
		s.setCertSummary(summary)
		return summary
	}
	pods, err := s.listAgentPods(ctx)
	if err != nil {
		summary := CertStatusSummary{Running: false, StartedAt: startedAt, CompletedAt: time.Now().Format(time.RFC3339), Error: err.Error(), NodeCount: len(nodes), Results: []CertNodeResult{}}
		s.setCertSummary(summary)
		return summary
	}

	agentsByNode := agentPodsByNode(pods)
	results := make([]CertNodeResult, 0, len(nodes))
	sourceErrors := make([]string, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range nodes {
		n := n
		agent, ok := agentsByNode[n.Metadata.Name]
		if !ok {
			result := certNodeResult(n, "", certNodeRole(n), certStatusAgentConnectError, "no k8s-tool-agent pod is running on this node")
			results = append(results, result)
			sourceErrors = append(sourceErrors, fmt.Sprintf("%s: %s", n.Metadata.Name, result.Status))
			continue
		}
		if agent.Status.Phase != "Running" || agent.Status.PodIP == "" {
			result := certNodeResult(n, "", certNodeRole(n), certStatusAgentConnectError, fmt.Sprintf("agent pod %s is not checkable: phase=%s podIP=%q", agent.Metadata.Name, agent.Status.Phase, agent.Status.PodIP))
			result.AgentPod = agent.Metadata.Name
			result.AgentPodIP = agent.Status.PodIP
			results = append(results, result)
			sourceErrors = append(sourceErrors, fmt.Sprintf("%s: %s", n.Metadata.Name, result.Message))
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.callAgentCertStatus(ctx, n, agent)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed := certNodeResult(n, "", certNodeRole(n), certStatusAgentConnectError, err.Error())
				failed.AgentPod = agent.Metadata.Name
				failed.AgentPodIP = agent.Status.PodIP
				failed.AgentURL = fmt.Sprintf("http://%s:%d/api/certs/status", agent.Status.PodIP, s.agentPort)
				results = append(results, failed)
				sourceErrors = append(sourceErrors, fmt.Sprintf("%s: %v", n.Metadata.Name, err))
				return
			}
			results = append(results, result)
			if result.Status != certStatusOK {
				sourceErrors = append(sourceErrors, fmt.Sprintf("%s: %s", n.Metadata.Name, result.Status))
			}
		}()
	}
	wg.Wait()

	summary := CertStatusSummary{
		Running:      false,
		StartedAt:    startedAt,
		CompletedAt:  time.Now().Format(time.RFC3339),
		NodeCount:    len(nodes),
		Results:      results,
		SourceErrors: sourceErrors,
	}
	finalizeCertSummary(&summary)
	if len(sourceErrors) > 0 {
		summary.Error = strings.Join(sourceErrors, "; ")
	}
	s.setCertSummary(summary)
	return summary
}

func (s *server) callAgentCertStatus(ctx context.Context, n node, agent pod) (CertNodeResult, error) {
	endpoint := fmt.Sprintf("http://%s:%d/api/certs/status", agent.Status.PodIP, s.agentPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return CertNodeResult{}, err
	}
	resp, err := (&http.Client{Timeout: s.certCheckTimeout + 5*time.Second}).Do(req)
	if err != nil {
		return CertNodeResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CertNodeResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CertNodeResult{}, fmt.Errorf("agent returned %s: %s", resp.Status, string(body))
	}
	var result CertNodeResult
	if err := json.Unmarshal(body, &result); err != nil {
		return CertNodeResult{}, err
	}
	if result.NodeName == "" {
		result.NodeName = n.Metadata.Name
	}
	if result.NodeIP == "" {
		result.NodeIP = nodeInternalIP(n)
	}
	result.Role = certNodeRole(n)
	result.AgentPod = agent.Metadata.Name
	result.AgentPodIP = agent.Status.PodIP
	result.AgentURL = endpoint
	for i := range result.Certificates {
		if result.Certificates[i].NodeName == "" {
			result.Certificates[i].NodeName = result.NodeName
		}
		if result.Certificates[i].NodeIP == "" {
			result.Certificates[i].NodeIP = result.NodeIP
		}
	}
	finalizeCertNodeResult(&result)
	return result, nil
}

func (s *server) listNodes(ctx context.Context) ([]node, error) {
	if s.kubeAPI == "" || s.token == "" {
		return nil, errors.New("Kubernetes service account environment is unavailable")
	}
	endpoint := fmt.Sprintf("%s/api/v1/nodes", s.kubeAPI)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.kubeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Kubernetes API returned %s: %s", resp.Status, string(body))
	}
	var list nodeList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func handleAgentCertStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info := collectAgentInfo()
	result := runAgentCertStatusCheck(info, envInt("CERT_EXPIRING_DAYS", 30))
	writeJSON(w, result)
}

func runAgentCertStatusCheck(info AgentInfo, expiringDays int) CertNodeResult {
	start := time.Now()
	result := CertNodeResult{
		NodeName:     info.NodeName,
		NodeIP:       info.NodeIP,
		AgentPod:     info.PodName,
		AgentPodIP:   info.PodIP,
		Status:       certStatusOK,
		CheckedAt:    start.Format(time.RFC3339),
		Certificates: []CertificateInfo{},
	}
	if err := probeHostNetworkNamespace(); err != nil {
		log.Printf("certificate status nsenter probe failed node=%s error=%v", info.NodeName, err)
	}
	serverRoot := env("RKE2_SERVER_TLS_DIR", "/var/lib/rancher/rke2/server/tls")
	agentRoot := env("RKE2_AGENT_DIR", "/var/lib/rancher/rke2/agent")

	serverExists := dirExists(serverRoot)
	agentExists := dirExists(agentRoot)
	if !serverExists && !agentExists {
		result.Status = certStatusMissingRKE2Path
		result.Message = fmt.Sprintf("RKE2 certificate paths not found: %s, %s", serverRoot, agentRoot)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	if serverExists {
		result.Certificates = append(result.Certificates, scanCertificates(info, serverRoot, "server", expiringDays, true)...)
	}
	if agentExists {
		result.Certificates = append(result.Certificates, scanCertificates(info, agentRoot, "agent", expiringDays, false)...)
	}
	sortCertsByExpiry(result.Certificates)
	finalizeCertNodeResult(&result)
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func probeHostNetworkNamespace() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "nsenter", "-t", "1", "-n", "true").Run()
}

func scanCertificates(info AgentInfo, root, category string, expiringDays int, recursive bool) []CertificateInfo {
	certs := []CertificateInfo{}
	addPath := func(path string) {
		if !isCertificateFile(path) || isPrivateKeyLike(path) {
			return
		}
		certs = append(certs, parseCertificateFile(info, category, path, expiringDays))
	}
	if recursive {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			addPath(path)
			return nil
		})
		return certs
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return certs
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		addPath(filepath.Join(root, entry.Name()))
	}
	return certs
}

func parseCertificateFile(info AgentInfo, category, path string, expiringDays int) CertificateInfo {
	now := time.Now()
	out := CertificateInfo{
		NodeName: info.NodeName,
		NodeIP:   info.NodeIP,
		Category: category,
		Path:     path,
		Status:   certStatusOK,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		out.ParseError = err.Error()
		out.Status = certStatusParseError
		return out
	}
	cert, err := parseFirstCertificate(data)
	if err != nil {
		out.ParseError = err.Error()
		out.Status = certStatusParseError
		return out
	}
	out.Subject = cert.Subject.String()
	out.Issuer = cert.Issuer.String()
	out.NotBefore = cert.NotBefore.Format(time.RFC3339)
	out.NotAfter = cert.NotAfter.Format(time.RFC3339)
	out.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	out.Expired = now.After(cert.NotAfter)
	out.ExpiringSoon = !out.Expired && out.DaysLeft <= expiringDays
	if out.Expired {
		out.Status = certStatusExpired
	} else if out.ExpiringSoon {
		out.Status = certStatusExpiringSoon
	}
	return out
}

func parseFirstCertificate(data []byte) (*x509.Certificate, error) {
	rest := data
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
		rest = remaining
	}
	return x509.ParseCertificate(data)
}

func isCertificateFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".crt" || ext == ".pem"
}

func isPrivateKeyLike(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return strings.Contains(name, "-key.") || strings.HasSuffix(name, ".key") || strings.Contains(name, "private-key")
}

func dirExists(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && stat.IsDir()
}

func certNodeRole(n node) string {
	labels := n.Metadata.Labels
	for _, key := range []string{"node-role.kubernetes.io/etcd", "node-role.kubernetes.io/control-plane", "node-role.kubernetes.io/master"} {
		if _, ok := labels[key]; ok {
			return "server"
		}
	}
	return "worker"
}

func certNodeResult(n node, agentURL, role, status, message string) CertNodeResult {
	return CertNodeResult{
		NodeName:     n.Metadata.Name,
		NodeIP:       nodeInternalIP(n),
		Role:         role,
		AgentURL:     agentURL,
		Status:       status,
		Message:      message,
		CheckedAt:    time.Now().Format(time.RFC3339),
		Certificates: []CertificateInfo{},
	}
}

func finalizeCertNodeResult(result *CertNodeResult) {
	result.ServerCertCount = 0
	result.AgentCertCount = 0
	result.ExpiredCount = 0
	result.ExpiringSoonCount = 0
	result.ParseErrorCount = 0
	sortCertsByExpiry(result.Certificates)
	for _, cert := range result.Certificates {
		switch cert.Category {
		case "server":
			result.ServerCertCount++
		case "agent":
			result.AgentCertCount++
		}
		if cert.Expired {
			result.ExpiredCount++
		}
		if cert.ExpiringSoon {
			result.ExpiringSoonCount++
		}
		if cert.ParseError != "" {
			result.ParseErrorCount++
		}
	}
	result.Status = certNodeStatus(*result)
	if result.Certificates == nil {
		result.Certificates = []CertificateInfo{}
	}
}

func certNodeStatus(result CertNodeResult) string {
	if result.Status == certStatusAgentConnectError || result.Status == certStatusMissingRKE2Path {
		return result.Status
	}
	if result.ParseErrorCount > 0 {
		return certStatusParseError
	}
	if result.ExpiredCount > 0 {
		return certStatusExpired
	}
	if result.ExpiringSoonCount > 0 {
		return certStatusExpiringSoon
	}
	return certStatusOK
}

func finalizeCertSummary(summary *CertStatusSummary) {
	summary.NodeCount = len(summary.Results)
	summary.ServerNodeCount = 0
	summary.WorkerNodeCount = 0
	summary.CheckedNodeCount = 0
	summary.TotalCertCount = 0
	summary.ExpiredCount = 0
	summary.ExpiringSoonCount = 0
	summary.ParseErrorCount = 0
	sort.Slice(summary.Results, func(i, j int) bool {
		return summary.Results[i].NodeName < summary.Results[j].NodeName
	})
	for i := range summary.Results {
		finalizeCertNodeResult(&summary.Results[i])
		switch summary.Results[i].Role {
		case "server":
			summary.ServerNodeCount++
		default:
			summary.WorkerNodeCount++
		}
		if summary.Results[i].Status != certStatusAgentConnectError {
			summary.CheckedNodeCount++
		}
		summary.TotalCertCount += len(summary.Results[i].Certificates)
		summary.ExpiredCount += summary.Results[i].ExpiredCount
		summary.ExpiringSoonCount += summary.Results[i].ExpiringSoonCount
		summary.ParseErrorCount += summary.Results[i].ParseErrorCount
	}
	if summary.Results == nil {
		summary.Results = []CertNodeResult{}
	}
}

func sortCertsByExpiry(certs []CertificateInfo) {
	sort.Slice(certs, func(i, j int) bool {
		if certs[i].NotAfter == "" {
			return false
		}
		if certs[j].NotAfter == "" {
			return true
		}
		return certs[i].NotAfter < certs[j].NotAfter
	})
}

func cloneCertSummary(summary CertStatusSummary) CertStatusSummary {
	summary.Results = append([]CertNodeResult(nil), summary.Results...)
	for i := range summary.Results {
		summary.Results[i].Certificates = append([]CertificateInfo(nil), summary.Results[i].Certificates...)
	}
	summary.SourceErrors = append([]string(nil), summary.SourceErrors...)
	if summary.Results == nil {
		summary.Results = []CertNodeResult{}
	}
	return summary
}

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AgentInfo struct {
	PodName       string `json:"podName"`
	Namespace     string `json:"namespace"`
	PodIP         string `json:"podIP"`
	NodeName      string `json:"nodeName"`
	NodeIP        string `json:"nodeIP"`
	Hostname      string `json:"hostname"`
	Phase         string `json:"phase"`
	AgentURL      string `json:"agentURL"`
	MemoryTotalKB int64  `json:"memoryTotalKB"`
	MemoryFreeKB  int64  `json:"memoryFreeKB"`
	MemoryUsedKB  int64  `json:"memoryUsedKB"`
	MemoryTotalGB string `json:"memoryTotalGB"`
	MemoryFreeGB  string `json:"memoryFreeGB"`
	MemoryUsedGB  string `json:"memoryUsedGB"`
	CollectedAt   string `json:"collectedAt"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	LastRefreshAt string `json:"lastRefreshAt"`
}

type NetworkTarget struct {
	PodName  string `json:"podName"`
	PodIP    string `json:"podIP"`
	NodeName string `json:"nodeName"`
	NodeIP   string `json:"nodeIP,omitempty"`
}

type NetworkCheckRequest struct {
	Targets []NetworkTarget `json:"targets"`
}

type LayeredNetworkTarget struct {
	Name     string `json:"name"`
	PodName  string `json:"podName,omitempty"`
	PodIP    string `json:"podIP,omitempty"`
	NodeName string `json:"nodeName"`
	NodeIP   string `json:"nodeIP,omitempty"`
	IP       string `json:"ip"`
	Kind     string `json:"kind"`
	HTTP     bool   `json:"http"`
}

type LayeredNetworkCheckRequest struct {
	Layer   string                 `json:"layer"`
	Targets []LayeredNetworkTarget `json:"targets"`
}

type NetworkCheckResponse struct {
	SourcePod  string               `json:"sourcePod"`
	SourceIP   string               `json:"sourceIP"`
	SourceNode string               `json:"sourceNode"`
	CheckedAt  string               `json:"checkedAt"`
	Results    []NetworkCheckResult `json:"results"`
}

type NetworkCheckResult struct {
	Layer          string `json:"layer,omitempty"`
	SourcePod      string `json:"sourcePod"`
	SourceNode     string `json:"sourceNode,omitempty"`
	SourceIP       string `json:"sourceIP,omitempty"`
	TargetName     string `json:"targetName,omitempty"`
	TargetPod      string `json:"targetPod"`
	TargetIP       string `json:"targetIP"`
	TargetNode     string `json:"targetNode"`
	PingOK         bool   `json:"pingOK"`
	PingDurationMS int64  `json:"pingDurationMS"`
	PingError      string `json:"pingError,omitempty"`
	HTTPOK         bool   `json:"httpOK"`
	HTTPStatus     int    `json:"httpStatus,omitempty"`
	HTTPDurationMS int64  `json:"httpDurationMS"`
	HTTPError      string `json:"httpError,omitempty"`
	CheckedAt      string `json:"checkedAt"`
	Skipped        bool   `json:"skipped,omitempty"`
	SkipReason     string `json:"skipReason,omitempty"`
}

type NetworkCheckSummary struct {
	Running      bool                 `json:"running"`
	StartedAt    string               `json:"startedAt,omitempty"`
	CompletedAt  string               `json:"completedAt,omitempty"`
	Error        string               `json:"error,omitempty"`
	AgentCount   int                  `json:"agentCount"`
	Results      []NetworkCheckResult `json:"results"`
	SourceErrors []string             `json:"sourceErrors,omitempty"`
}

type NetworkStats struct {
	AgentCount   int `json:"agentCount"`
	TotalChecks  int `json:"totalChecks"`
	SuccessCount int `json:"successCount"`
	FailedCount  int `json:"failedCount"`
	SkippedCount int `json:"skippedCount"`
	PingFailed   int `json:"pingFailed"`
	HTTPFailed   int `json:"httpFailed"`
}

type NetworkSourceSummary struct {
	SourcePod    string               `json:"sourcePod"`
	TargetCount  int                  `json:"targetCount"`
	SuccessCount int                  `json:"successCount"`
	FailedCount  int                  `json:"failedCount"`
	SkippedCount int                  `json:"skippedCount"`
	PingOKCount  int                  `json:"pingOKCount"`
	HTTPOKCount  int                  `json:"httpOKCount"`
	Failures     []NetworkCheckResult `json:"failures"`
}

type NetworkView struct {
	Stats    NetworkStats           `json:"stats"`
	Sources  []NetworkSourceSummary `json:"sources"`
	Failures []NetworkCheckResult   `json:"failures"`
}

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata metadata  `json:"metadata"`
	Spec     podSpec   `json:"spec"`
	Status   podStatus `json:"status"`
}

type metadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type podStatus struct {
	PodIP  string `json:"podIP"`
	HostIP string `json:"hostIP"`
	Phase  string `json:"phase"`
}

type podSpec struct {
	NodeName string `json:"nodeName"`
}

type server struct {
	namespace       string
	agentSelector   string
	agentPort       int
	refreshInterval time.Duration
	kubeAPI         string
	token           string
	httpClient      *http.Client
	kubeClient      *http.Client
	mu              sync.RWMutex
	agents          []AgentInfo
	lastError       string
	network         NetworkCheckSummary
	layeredNetwork  NetworkCheckSummary
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "agent" {
		runAgent()
		return
	}

	srv, err := newServer()
	if err != nil {
		log.Fatalf("server init failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.refresh(ctx)
	go srv.refreshLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents", srv.handleAgents)
	mux.HandleFunc("/api/refresh", srv.handleRefresh)
	mux.HandleFunc("/api/network-check", srv.handleNetworkCheck)
	mux.HandleFunc("/api/layered-network-check", srv.handleLayeredNetworkCheck)
	mux.HandleFunc("/", handleFrontend)

	addr := ":80"
	log.Printf("k8s-tool-server listening on %s namespace=%s selector=%q agentPort=%d refresh=%s", addr, srv.namespace, srv.agentSelector, srv.agentPort, srv.refreshInterval)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func newServer() (*server, error) {
	namespace := env("POD_NAMESPACE", readFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace", "default"))
	refreshSeconds := envInt("REFRESH_INTERVAL_SECONDS", 10)
	agentPort := envInt("AGENT_PORT", 80)

	tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		log.Printf("service account token unavailable; Kubernetes discovery will return an empty list: %v", err)
	}

	kubeHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubePort := env("KUBERNETES_SERVICE_PORT", "443")
	kubeAPI := ""
	if kubeHost != "" {
		kubeAPI = "https://" + netJoinHostPort(kubeHost, kubePort)
	}

	kubeClient := &http.Client{Timeout: 5 * time.Second}
	caPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	if caCert, err := os.ReadFile(caPath); err == nil {
		roots := x509.NewCertPool()
		if roots.AppendCertsFromPEM(caCert) {
			kubeClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
		}
	}

	return &server{
		namespace:       namespace,
		agentSelector:   env("AGENT_SELECTOR", "app.kubernetes.io/name=k8s-tool,app.kubernetes.io/component=agent"),
		agentPort:       agentPort,
		refreshInterval: time.Duration(refreshSeconds) * time.Second,
		kubeAPI:         kubeAPI,
		token:           strings.TrimSpace(string(tokenBytes)),
		httpClient:      &http.Client{Timeout: 3 * time.Second},
		kubeClient:      kubeClient,
		agents:          []AgentInfo{},
		network:         NetworkCheckSummary{Results: []NetworkCheckResult{}},
		layeredNetwork:  NetworkCheckSummary{Results: []NetworkCheckResult{}},
	}, nil
}

func (s *server) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.refresh(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *server) refresh(ctx context.Context) {
	log.Printf("agent refresh started")
	agents, err := s.fetchAgents(ctx)
	errText := ""
	if err != nil {
		errText = err.Error()
		log.Printf("agent refresh failed: %v", err)
	}
	if agents == nil {
		agents = []AgentInfo{}
	}
	s.mu.Lock()
	s.agents = agents
	s.lastError = errText
	s.mu.Unlock()
	log.Printf("agent refresh completed agents=%d error=%q", len(agents), errText)
}

func (s *server) fetchAgents(ctx context.Context) ([]AgentInfo, error) {
	pods, err := s.listAgentPods(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("discovered agent pods=%d", len(pods))

	results := make([]AgentInfo, len(pods))
	var wg sync.WaitGroup
	for i, pod := range pods {
		i, pod := i, pod
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = s.fetchAgentInfo(ctx, pod)
		}()
	}
	wg.Wait()
	return results, nil
}

func (s *server) listAgentPods(ctx context.Context) ([]pod, error) {
	if s.kubeAPI == "" || s.token == "" {
		return nil, errors.New("Kubernetes service account environment is unavailable")
	}

	endpoint := fmt.Sprintf("%s/api/v1/namespaces/%s/pods?labelSelector=%s", s.kubeAPI, url.PathEscape(s.namespace), url.QueryEscape(s.agentSelector))
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

	var list podList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s *server) fetchAgentInfo(ctx context.Context, pod pod) AgentInfo {
	now := time.Now().Format(time.RFC3339)
	info := AgentInfo{
		PodName:       pod.Metadata.Name,
		Namespace:     pod.Metadata.Namespace,
		PodIP:         pod.Status.PodIP,
		NodeName:      pod.Spec.NodeName,
		NodeIP:        pod.Status.HostIP,
		Phase:         pod.Status.Phase,
		Status:        "connect-error",
		LastRefreshAt: now,
	}
	if pod.Status.Phase != "Running" {
		info.Status = "pod-not-running"
		info.Error = "agent pod phase is " + pod.Status.Phase
		log.Printf("agent unavailable pod=%s phase=%s", pod.Metadata.Name, pod.Status.Phase)
		return info.withMemoryGB()
	}
	if pod.Status.PodIP == "" {
		info.Status = "missing-pod-ip"
		info.Error = "agent pod has no pod IP"
		log.Printf("agent unavailable pod=%s reason=missing-pod-ip", pod.Metadata.Name)
		return info.withMemoryGB()
	}

	endpoint := fmt.Sprintf("http://%s:%d/api/node-info", pod.Status.PodIP, s.agentPort)
	info.AgentURL = endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		info.Error = err.Error()
		log.Printf("agent request build failed pod=%s error=%v", pod.Metadata.Name, err)
		return info.withMemoryGB()
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		log.Printf("agent fetch failed pod=%s url=%s error=%v", pod.Metadata.Name, endpoint, err)
		return info.withMemoryGB()
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		info.Error = err.Error()
		log.Printf("agent response read failed pod=%s error=%v", pod.Metadata.Name, err)
		return info.withMemoryGB()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		info.Status = "http-error"
		info.Error = fmt.Sprintf("agent returned %s: %s", resp.Status, string(body))
		log.Printf("agent http error pod=%s status=%s body=%q", pod.Metadata.Name, resp.Status, string(body))
		return info.withMemoryGB()
	}
	if err := json.Unmarshal(body, &info); err != nil {
		info = AgentInfo{
			PodName:       pod.Metadata.Name,
			Namespace:     pod.Metadata.Namespace,
			PodIP:         pod.Status.PodIP,
			NodeName:      pod.Spec.NodeName,
			NodeIP:        pod.Status.HostIP,
			Phase:         pod.Status.Phase,
			AgentURL:      endpoint,
			Status:        "invalid-json",
			Error:         err.Error(),
			LastRefreshAt: now,
		}
		log.Printf("agent json decode failed pod=%s error=%v body=%q", pod.Metadata.Name, err, string(body))
		return info.withMemoryGB()
	}
	info.Phase = pod.Status.Phase
	info.AgentURL = endpoint
	if info.NodeIP == "" {
		info.NodeIP = pod.Status.HostIP
	}
	info.Status = "online"
	info.LastRefreshAt = now
	log.Printf("agent fetch ok pod=%s node=%s podIP=%s", info.PodName, info.NodeName, info.PodIP)
	return info.withMemoryGB()
}

func (a AgentInfo) withMemoryGB() AgentInfo {
	a.MemoryTotalGB = kbToGiB(a.MemoryTotalKB)
	a.MemoryFreeGB = kbToGiB(a.MemoryFreeKB)
	a.MemoryUsedGB = kbToGiB(a.MemoryUsedKB)
	return a
}

func (s *server) runNetworkCheck(ctx context.Context) NetworkCheckSummary {
	log.Printf("network check started")
	startedAt := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	s.network.Running = true
	s.network.StartedAt = startedAt
	s.network.CompletedAt = ""
	s.network.Error = ""
	s.mu.Unlock()

	pods, err := s.listAgentPods(ctx)
	if err != nil {
		summary := NetworkCheckSummary{Running: false, StartedAt: startedAt, CompletedAt: time.Now().Format(time.RFC3339), Error: err.Error(), Results: []NetworkCheckResult{}}
		s.setNetworkSummary(summary)
		log.Printf("network check failed discovery error=%v", err)
		return summary
	}

	targets := make([]NetworkTarget, 0, len(pods))
	for _, pod := range pods {
		if pod.Status.Phase == "Running" && pod.Status.PodIP != "" {
			targets = append(targets, NetworkTarget{PodName: pod.Metadata.Name, PodIP: pod.Status.PodIP, NodeName: pod.Spec.NodeName, NodeIP: pod.Status.HostIP})
		}
	}

	results := make([]NetworkCheckResult, 0)
	sourceErrors := make([]string, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, source := range pods {
		source := source
		if source.Status.Phase != "Running" || source.Status.PodIP == "" {
			mu.Lock()
			results = append(results, skippedResultsForSource(source, targets)...)
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sourceResults, err := s.callAgentNetworkCheck(ctx, source, targets)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				sourceErrors = append(sourceErrors, fmt.Sprintf("%s: %v", source.Metadata.Name, err))
				results = append(results, failedResultsForSource(source, targets, err.Error())...)
				log.Printf("network check source failed pod=%s error=%v", source.Metadata.Name, err)
				return
			}
			results = append(results, sourceResults...)
			log.Printf("network check source ok pod=%s results=%d", source.Metadata.Name, len(sourceResults))
		}()
	}
	wg.Wait()

	summary := NetworkCheckSummary{
		Running:      false,
		StartedAt:    startedAt,
		CompletedAt:  time.Now().Format(time.RFC3339),
		AgentCount:   len(pods),
		Results:      results,
		SourceErrors: sourceErrors,
	}
	if len(sourceErrors) > 0 {
		summary.Error = strings.Join(sourceErrors, "; ")
	}
	s.setNetworkSummary(summary)
	log.Printf("network check completed agents=%d results=%d errors=%d", len(pods), len(results), len(sourceErrors))
	return summary
}

func (s *server) runLayeredNetworkCheck(ctx context.Context) NetworkCheckSummary {
	log.Printf("layered network check started")
	startedAt := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	s.layeredNetwork.Running = true
	s.layeredNetwork.StartedAt = startedAt
	s.layeredNetwork.CompletedAt = ""
	s.layeredNetwork.Error = ""
	s.mu.Unlock()

	pods, err := s.listAgentPods(ctx)
	if err != nil {
		summary := NetworkCheckSummary{Running: false, StartedAt: startedAt, CompletedAt: time.Now().Format(time.RFC3339), Error: err.Error(), Results: []NetworkCheckResult{}}
		s.setLayeredNetworkSummary(summary)
		log.Printf("layered network check failed discovery error=%v", err)
		return summary
	}

	layerTargets := buildLayerTargets(pods)
	layerOrder := []string{"pod-to-pod", "node-to-node", "node-to-pod", "pod-to-node"}
	results := make([]NetworkCheckResult, 0)
	sourceErrors := make([]string, 0)
	var mu sync.Mutex

	for _, layer := range layerOrder {
		targets := layerTargets[layer]
		log.Printf("layered network check layer=%s sources=%d targets=%d", layer, len(pods), len(targets))
		var wg sync.WaitGroup
		for _, source := range pods {
			source := source
			layer := layer
			targets := targets
			if !sourceCheckableForLayer(source, layer) {
				mu.Lock()
				results = append(results, skippedLayerResultsForSource(layer, source, targets)...)
				mu.Unlock()
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				sourceResults, err := s.callAgentLayeredNetworkCheck(ctx, layer, source, targets)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					sourceErrors = append(sourceErrors, fmt.Sprintf("%s/%s: %v", layer, source.Metadata.Name, err))
					results = append(results, failedLayerResultsForSource(layer, source, targets, err.Error())...)
					log.Printf("layered network check source failed layer=%s pod=%s error=%v", layer, source.Metadata.Name, err)
					return
				}
				results = append(results, sourceResults...)
				log.Printf("layered network check source ok layer=%s pod=%s results=%d", layer, source.Metadata.Name, len(sourceResults))
			}()
		}
		wg.Wait()
	}

	summary := NetworkCheckSummary{
		Running:      false,
		StartedAt:    startedAt,
		CompletedAt:  time.Now().Format(time.RFC3339),
		AgentCount:   len(pods),
		Results:      results,
		SourceErrors: sourceErrors,
	}
	if len(sourceErrors) > 0 {
		summary.Error = strings.Join(sourceErrors, "; ")
	}
	s.setLayeredNetworkSummary(summary)
	log.Printf("layered network check completed agents=%d results=%d errors=%d", len(pods), len(results), len(sourceErrors))
	return summary
}

func buildLayerTargets(pods []pod) map[string][]LayeredNetworkTarget {
	podTargets := make([]LayeredNetworkTarget, 0, len(pods))
	nodeTargets := make([]LayeredNetworkTarget, 0, len(pods))
	seenNodes := map[string]bool{}
	for _, pod := range pods {
		if pod.Status.Phase == "Running" && pod.Status.PodIP != "" {
			podTargets = append(podTargets, LayeredNetworkTarget{
				Name:     pod.Metadata.Name,
				PodName:  pod.Metadata.Name,
				PodIP:    pod.Status.PodIP,
				NodeName: pod.Spec.NodeName,
				NodeIP:   pod.Status.HostIP,
				IP:       pod.Status.PodIP,
				Kind:     "pod",
				HTTP:     true,
			})
		}
		if pod.Spec.NodeName != "" && pod.Status.HostIP != "" && !seenNodes[pod.Spec.NodeName] {
			seenNodes[pod.Spec.NodeName] = true
			nodeTargets = append(nodeTargets, LayeredNetworkTarget{
				Name:     pod.Spec.NodeName,
				NodeName: pod.Spec.NodeName,
				NodeIP:   pod.Status.HostIP,
				IP:       pod.Status.HostIP,
				Kind:     "node",
			})
		}
	}
	return map[string][]LayeredNetworkTarget{
		"pod-to-pod":   podTargets,
		"node-to-node": nodeTargets,
		"node-to-pod":  podTargets,
		"pod-to-node":  nodeTargets,
	}
}

func sourceCheckableForLayer(source pod, layer string) bool {
	if source.Status.Phase != "Running" || source.Status.PodIP == "" {
		return false
	}
	if strings.HasPrefix(layer, "node-to-") && source.Status.HostIP == "" {
		return false
	}
	return true
}

func (s *server) callAgentLayeredNetworkCheck(ctx context.Context, layer string, source pod, targets []LayeredNetworkTarget) ([]NetworkCheckResult, error) {
	endpoint := fmt.Sprintf("http://%s:%d/api/layered-network-check", source.Status.PodIP, s.agentPort)
	payload, err := json.Marshal(LayeredNetworkCheckRequest{Layer: layer, Targets: targets})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("agent returned %s: %s", resp.Status, string(body))
	}
	var agentResponse NetworkCheckResponse
	if err := json.Unmarshal(body, &agentResponse); err != nil {
		return nil, err
	}
	return agentResponse.Results, nil
}

func (s *server) callAgentNetworkCheck(ctx context.Context, source pod, targets []NetworkTarget) ([]NetworkCheckResult, error) {
	endpoint := fmt.Sprintf("http://%s:%d/api/network-check", source.Status.PodIP, s.agentPort)
	payload, err := json.Marshal(NetworkCheckRequest{Targets: targets})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("agent returned %s: %s", resp.Status, string(body))
	}
	var agentResponse NetworkCheckResponse
	if err := json.Unmarshal(body, &agentResponse); err != nil {
		return nil, err
	}
	return agentResponse.Results, nil
}

func skippedResultsForSource(source pod, targets []NetworkTarget) []NetworkCheckResult {
	reason := "source pod is not checkable"
	if source.Status.Phase != "Running" {
		reason = "source pod phase is " + source.Status.Phase
	} else if source.Status.PodIP == "" {
		reason = "source pod has no pod IP"
	}
	results := make([]NetworkCheckResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, NetworkCheckResult{
			SourcePod:  source.Metadata.Name,
			SourceNode: source.Spec.NodeName,
			SourceIP:   source.Status.PodIP,
			TargetName: target.PodName,
			TargetPod:  target.PodName,
			TargetIP:   target.PodIP,
			TargetNode: target.NodeName,
			Skipped:    true,
			SkipReason: reason,
			CheckedAt:  time.Now().Format(time.RFC3339),
		})
	}
	return results
}

func failedResultsForSource(source pod, targets []NetworkTarget, reason string) []NetworkCheckResult {
	results := make([]NetworkCheckResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, NetworkCheckResult{
			SourcePod:  source.Metadata.Name,
			SourceNode: source.Spec.NodeName,
			SourceIP:   source.Status.PodIP,
			TargetName: target.PodName,
			TargetPod:  target.PodName,
			TargetIP:   target.PodIP,
			TargetNode: target.NodeName,
			HTTPError:  reason,
			PingError:  reason,
			CheckedAt:  time.Now().Format(time.RFC3339),
		})
	}
	return results
}

func skippedLayerResultsForSource(layer string, source pod, targets []LayeredNetworkTarget) []NetworkCheckResult {
	reason := "source pod is not checkable"
	if source.Status.Phase != "Running" {
		reason = "source pod phase is " + source.Status.Phase
	} else if source.Status.PodIP == "" {
		reason = "source pod has no pod IP"
	} else if strings.HasPrefix(layer, "node-to-") && source.Status.HostIP == "" {
		reason = "source node has no host IP"
	}
	results := make([]NetworkCheckResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, NetworkCheckResult{
			Layer:      layer,
			SourcePod:  source.Metadata.Name,
			SourceNode: source.Spec.NodeName,
			SourceIP:   sourceIPForLayer(source, layer),
			TargetName: target.Name,
			TargetPod:  target.PodName,
			TargetIP:   target.IP,
			TargetNode: target.NodeName,
			Skipped:    true,
			SkipReason: reason,
			CheckedAt:  time.Now().Format(time.RFC3339),
		})
	}
	return results
}

func failedLayerResultsForSource(layer string, source pod, targets []LayeredNetworkTarget, reason string) []NetworkCheckResult {
	results := make([]NetworkCheckResult, 0, len(targets))
	for _, target := range targets {
		results = append(results, NetworkCheckResult{
			Layer:      layer,
			SourcePod:  source.Metadata.Name,
			SourceNode: source.Spec.NodeName,
			SourceIP:   sourceIPForLayer(source, layer),
			TargetName: target.Name,
			TargetPod:  target.PodName,
			TargetIP:   target.IP,
			TargetNode: target.NodeName,
			HTTPError:  reason,
			PingError:  reason,
			CheckedAt:  time.Now().Format(time.RFC3339),
		})
	}
	return results
}

func sourceIPForLayer(source pod, layer string) string {
	if strings.HasPrefix(layer, "node-to-") {
		return source.Status.HostIP
	}
	return source.Status.PodIP
}

func runAgent() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleAgentIndex)
	mux.HandleFunc("/api/node-info", handleAgentNodeInfo)
	mux.HandleFunc("/api/network-check", handleAgentNetworkCheck)
	mux.HandleFunc("/api/layered-network-check", handleAgentLayeredNetworkCheck)
	addr := ":80"
	log.Printf("k8s-tool-agent listening on %s pod=%s namespace=%s podIP=%s node=%s hostIP=%s", addr, os.Getenv("POD_NAME"), os.Getenv("POD_NAMESPACE"), os.Getenv("POD_IP"), os.Getenv("NODE_NAME"), os.Getenv("HOST_IP"))
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleAgentIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	info := collectAgentInfo()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>k8s-tool-agent</title>
  <style>
    :root { color-scheme: light dark; font-family: Arial, Helvetica, sans-serif; background: #f6f8fb; color: #17202a; }
    body { margin: 0; padding: 24px; }
    main { max-width: 760px; }
    h1 { margin: 0 0 16px; font-size: 26px; letter-spacing: 0; }
    dl { display: grid; grid-template-columns: 150px 1fr; gap: 10px 14px; background: #ffffff; border: 1px solid #d9e2ec; border-radius: 8px; padding: 16px; }
    dt { color: #52606d; font-weight: 700; }
    dd { margin: 0; word-break: break-word; }
    @media (prefers-color-scheme: dark) { :root { background: #121820; color: #eef2f6; } dl { background: #1c2633; border-color: #34445a; } dt { color: #b7c4d3; } }
  </style>
</head>
<body>
  <main>
    <h1>k8s-tool-agent</h1>
    <dl>
      <dt>Pod Name</dt><dd>%s</dd>
      <dt>Pod IP</dt><dd>%s</dd>
      <dt>Node IP</dt><dd>%s</dd>
      <dt>Collected</dt><dd>%s</dd>
    </dl>
  </main>
</body>
</html>`, template.HTMLEscapeString(info.PodName), template.HTMLEscapeString(info.PodIP), template.HTMLEscapeString(info.NodeIP), template.HTMLEscapeString(info.CollectedAt))
}

func handleAgentNodeInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info := collectAgentInfo()
	log.Printf("node info requested pod=%s podIP=%s node=%s", info.PodName, info.PodIP, info.NodeName)
	writeJSON(w, info)
}

func handleAgentNetworkCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req NetworkCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info := collectAgentInfo()
	log.Printf("network check requested source=%s targets=%d", info.PodName, len(req.Targets))
	results := make([]NetworkCheckResult, 0, len(req.Targets))
	for _, target := range req.Targets {
		results = append(results, runTargetCheck(info, target))
	}
	log.Printf("network check completed source=%s results=%d", info.PodName, len(results))
	writeJSON(w, NetworkCheckResponse{
		SourcePod:  info.PodName,
		SourceIP:   info.PodIP,
		SourceNode: info.NodeName,
		CheckedAt:  time.Now().Format(time.RFC3339),
		Results:    results,
	})
}

func handleAgentLayeredNetworkCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LayeredNetworkCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validNetworkLayer(req.Layer) {
		http.Error(w, "invalid layer", http.StatusBadRequest)
		return
	}
	info := collectAgentInfo()
	log.Printf("layered network check requested source=%s layer=%s targets=%d", info.PodName, req.Layer, len(req.Targets))
	results := make([]NetworkCheckResult, len(req.Targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i, target := range req.Targets {
		i, target := i, target
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = runLayeredTargetCheck(info, req.Layer, target)
		}()
	}
	wg.Wait()
	log.Printf("layered network check completed source=%s layer=%s results=%d", info.PodName, req.Layer, len(results))
	writeJSON(w, NetworkCheckResponse{
		SourcePod:  info.PodName,
		SourceIP:   sourceIPForAgentLayer(info, req.Layer),
		SourceNode: info.NodeName,
		CheckedAt:  time.Now().Format(time.RFC3339),
		Results:    results,
	})
}

func collectAgentInfo() AgentInfo {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	memTotal := memInfoKB("MemTotal")
	memAvailable := memInfoKB("MemAvailable")
	memUsed := int64(0)
	if memTotal > 0 && memAvailable >= 0 {
		memUsed = memTotal - memAvailable
	}
	return AgentInfo{
		PodName:       env("POD_NAME", hostname),
		Namespace:     env("POD_NAMESPACE", "default"),
		PodIP:         env("POD_IP", "unknown"),
		NodeName:      env("NODE_NAME", "unknown"),
		NodeIP:        env("HOST_IP", "unknown"),
		Hostname:      hostname,
		MemoryTotalKB: memTotal,
		MemoryFreeKB:  memAvailable,
		MemoryUsedKB:  memUsed,
		CollectedAt:   time.Now().Format(time.RFC3339),
	}.withMemoryGB()
}

func memInfoKB(key string) int64 {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("read /proc/meminfo failed: %v", err)
		return 0
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimSuffix(fields[0], ":") == key {
			value, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return value
		}
	}
	return 0
}

func runTargetCheck(source AgentInfo, target NetworkTarget) NetworkCheckResult {
	result := NetworkCheckResult{
		Layer:      "pod-to-pod",
		SourcePod:  source.PodName,
		SourceNode: source.NodeName,
		SourceIP:   source.PodIP,
		TargetName: target.PodName,
		TargetPod:  target.PodName,
		TargetIP:   target.PodIP,
		TargetNode: target.NodeName,
		CheckedAt:  time.Now().Format(time.RFC3339),
	}
	if target.PodIP == "" {
		result.Skipped = true
		result.SkipReason = "target pod has no pod IP"
		return result
	}

	pingStart := time.Now()
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	pingOutput, pingErr := exec.CommandContext(pingCtx, "ping", "-c", "1", "-W", "2", target.PodIP).CombinedOutput()
	pingCancel()
	result.PingDurationMS = time.Since(pingStart).Milliseconds()
	if pingErr != nil {
		result.PingError = strings.TrimSpace(string(pingOutput))
		if result.PingError == "" {
			result.PingError = pingErr.Error()
		}
	} else {
		result.PingOK = true
	}

	httpStart := time.Now()
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 3*time.Second)
	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, fmt.Sprintf("http://%s:80/api/node-info", target.PodIP), nil)
	if err == nil {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			result.HTTPError = err.Error()
		} else {
			result.HTTPStatus = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			result.HTTPOK = resp.StatusCode >= 200 && resp.StatusCode < 300
			if !result.HTTPOK {
				result.HTTPError = fmt.Sprintf("HTTP status %d", resp.StatusCode)
			}
		}
	} else {
		result.HTTPError = err.Error()
	}
	httpCancel()
	result.HTTPDurationMS = time.Since(httpStart).Milliseconds()
	return result
}

func runLayeredTargetCheck(source AgentInfo, layer string, target LayeredNetworkTarget) NetworkCheckResult {
	result := NetworkCheckResult{
		Layer:      layer,
		SourcePod:  source.PodName,
		SourceNode: source.NodeName,
		SourceIP:   sourceIPForAgentLayer(source, layer),
		TargetName: target.Name,
		TargetPod:  target.PodName,
		TargetIP:   target.IP,
		TargetNode: target.NodeName,
		CheckedAt:  time.Now().Format(time.RFC3339),
	}
	if target.IP == "" {
		result.Skipped = true
		result.SkipReason = "target has no IP"
		return result
	}

	useHostNet := strings.HasPrefix(layer, "node-to-")
	runPingCheck(&result, target.IP, useHostNet)
	if target.HTTP {
		runHTTPCheck(&result, target.IP, useHostNet)
	}
	return result
}

func runPingCheck(result *NetworkCheckResult, targetIP string, useHostNet bool) {
	pingStart := time.Now()
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	pingOutput, pingErr := runNetworkCommand(pingCtx, useHostNet, "ping", "-c", "1", "-W", "2", targetIP)
	pingCancel()
	result.PingDurationMS = time.Since(pingStart).Milliseconds()
	if pingErr != nil {
		result.PingError = strings.TrimSpace(string(pingOutput))
		if result.PingError == "" {
			result.PingError = pingErr.Error()
		}
		return
	}
	result.PingOK = true
}

func runHTTPCheck(result *NetworkCheckResult, targetIP string, useHostNet bool) {
	httpStart := time.Now()
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer httpCancel()
	if useHostNet {
		output, err := runNetworkCommand(httpCtx, true, "curl", "-fsS", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "3", fmt.Sprintf("http://%s:80/api/node-info", targetIP))
		result.HTTPDurationMS = time.Since(httpStart).Milliseconds()
		if err != nil {
			result.HTTPError = strings.TrimSpace(string(output))
			if result.HTTPError == "" {
				result.HTTPError = err.Error()
			}
			return
		}
		status, err := strconv.Atoi(strings.TrimSpace(string(output)))
		if err != nil {
			result.HTTPError = "invalid curl status: " + strings.TrimSpace(string(output))
			return
		}
		result.HTTPStatus = status
		result.HTTPOK = status >= 200 && status < 300
		if !result.HTTPOK {
			result.HTTPError = fmt.Sprintf("HTTP status %d", status)
		}
		return
	}

	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, fmt.Sprintf("http://%s:80/api/node-info", targetIP), nil)
	if err == nil {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			result.HTTPError = err.Error()
		} else {
			result.HTTPStatus = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			result.HTTPOK = resp.StatusCode >= 200 && resp.StatusCode < 300
			if !result.HTTPOK {
				result.HTTPError = fmt.Sprintf("HTTP status %d", resp.StatusCode)
			}
		}
	} else {
		result.HTTPError = err.Error()
	}
	result.HTTPDurationMS = time.Since(httpStart).Milliseconds()
}

func runNetworkCommand(ctx context.Context, useHostNet bool, name string, args ...string) ([]byte, error) {
	if useHostNet {
		nsenterArgs := append([]string{"-t", "1", "-n", name}, args...)
		return exec.CommandContext(ctx, "nsenter", nsenterArgs...).CombinedOutput()
	}
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func validNetworkLayer(layer string) bool {
	switch layer {
	case "pod-to-pod", "node-to-node", "node-to-pod", "pod-to-node":
		return true
	default:
		return false
	}
}

func sourceIPForAgentLayer(source AgentInfo, layer string) string {
	if strings.HasPrefix(layer, "node-to-") {
		return source.NodeIP
	}
	return source.PodIP
}

func (s *server) setNetworkSummary(summary NetworkCheckSummary) {
	if summary.Results == nil {
		summary.Results = []NetworkCheckResult{}
	}
	s.mu.Lock()
	s.network = summary
	s.mu.Unlock()
}

func (s *server) setLayeredNetworkSummary(summary NetworkCheckSummary) {
	if summary.Results == nil {
		summary.Results = []NetworkCheckResult{}
	}
	s.mu.Lock()
	s.layeredNetwork = summary
	s.mu.Unlock()
}

func (s *server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeAgents(w)
}

func (s *server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	s.refresh(ctx)
	s.writeAgents(w)
}

func (s *server) handleNetworkCheck(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeNetwork(w)
	case http.MethodPost:
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		summary := s.runNetworkCheck(ctx)
		writeJSON(w, summary)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleLayeredNetworkCheck(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeLayeredNetwork(w)
	case http.MethodPost:
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		summary := s.runLayeredNetworkCheck(ctx)
		writeJSON(w, summary)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleFrontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	root := frontendRoot()
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	fullPath := root + "/" + path
	if _, err := os.Stat(fullPath); err != nil {
		fullPath = root + "/index.html"
	}
	http.ServeFile(w, r, fullPath)
}

func frontendRoot() string {
	if root := strings.TrimSpace(os.Getenv("FRONTEND_DIR")); root != "" {
		return root
	}
	if _, err := os.Stat("/usr/local/share/k8s-tool/frontend/index.html"); err == nil {
		return "/usr/local/share/k8s-tool/frontend"
	}
	return "frontend/dist"
}

func (s *server) writeAgents(w http.ResponseWriter) {
	s.mu.RLock()
	data := struct {
		Agents    []AgentInfo `json:"agents"`
		LastError string      `json:"lastError,omitempty"`
	}{
		Agents:    cloneAgents(s.agents),
		LastError: s.lastError,
	}
	s.mu.RUnlock()
	writeJSON(w, data)
}

func (s *server) writeNetwork(w http.ResponseWriter) {
	s.mu.RLock()
	data := cloneNetworkSummary(s.network)
	s.mu.RUnlock()
	writeJSON(w, data)
}

func (s *server) writeLayeredNetwork(w http.ResponseWriter) {
	s.mu.RLock()
	data := cloneNetworkSummary(s.layeredNetwork)
	s.mu.RUnlock()
	writeJSON(w, data)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func cloneAgents(agents []AgentInfo) []AgentInfo {
	out := make([]AgentInfo, len(agents))
	copy(out, agents)
	return out
}

func cloneNetworkSummary(summary NetworkCheckSummary) NetworkCheckSummary {
	summary.Results = append([]NetworkCheckResult(nil), summary.Results...)
	summary.SourceErrors = append([]string(nil), summary.SourceErrors...)
	if summary.Results == nil {
		summary.Results = []NetworkCheckResult{}
	}
	return summary
}

func buildNetworkView(summary NetworkCheckSummary) NetworkView {
	view := NetworkView{
		Stats: NetworkStats{
			AgentCount:  summary.AgentCount,
			TotalChecks: len(summary.Results),
		},
		Sources:  []NetworkSourceSummary{},
		Failures: []NetworkCheckResult{},
	}

	bySource := map[string]*NetworkSourceSummary{}
	sourceOrder := []string{}
	for _, result := range summary.Results {
		source := result.SourcePod
		if source == "" {
			source = "unknown"
		}
		if _, ok := bySource[source]; !ok {
			sourceOrder = append(sourceOrder, source)
			bySource[source] = &NetworkSourceSummary{SourcePod: source, Failures: []NetworkCheckResult{}}
		}
		row := bySource[source]
		row.TargetCount++
		if result.Skipped {
			row.SkippedCount++
			view.Stats.SkippedCount++
		}
		if result.PingOK {
			row.PingOKCount++
		}
		if result.HTTPOK {
			row.HTTPOKCount++
		}
		if networkResultOK(result) {
			row.SuccessCount++
			view.Stats.SuccessCount++
			continue
		}
		row.FailedCount++
		row.Failures = append(row.Failures, result)
		view.Failures = append(view.Failures, result)
		view.Stats.FailedCount++
		if !result.Skipped && !result.PingOK {
			view.Stats.PingFailed++
		}
		if !result.Skipped && httpRequired(result) && !result.HTTPOK {
			view.Stats.HTTPFailed++
		}
	}

	for _, source := range sourceOrder {
		view.Sources = append(view.Sources, *bySource[source])
	}
	return view
}

func networkResultOK(result NetworkCheckResult) bool {
	return !result.Skipped && result.PingOK && (!httpRequired(result) || result.HTTPOK)
}

func httpRequired(result NetworkCheckResult) bool {
	return result.Layer == "" || result.Layer == "pod-to-pod" || result.Layer == "node-to-pod"
}

func kbToGiB(kb int64) string {
	if kb <= 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", float64(kb)/1024.0/1024.0)
}

func env(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func readFile(path, fallback string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(content))
}

func netJoinHostPort(host, port string) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

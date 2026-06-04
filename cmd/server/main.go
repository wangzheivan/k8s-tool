package main

import (
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
	MemoryTotalKB int64  `json:"memoryTotalKB"`
	MemoryFreeKB  int64  `json:"memoryFreeKB"`
	MemoryUsedKB  int64  `json:"memoryUsedKB"`
	CollectedAt   string `json:"collectedAt"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	LastRefreshAt string `json:"lastRefreshAt"`
}

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata metadata  `json:"metadata"`
	Status   podStatus `json:"status"`
}

type metadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type podStatus struct {
	PodIP string `json:"podIP"`
	Phase string `json:"phase"`
}

type server struct {
	namespace       string
	agentSelector   string
	agentPort       int
	refreshInterval time.Duration
	kubeAPI         string
	token           string
	caPath          string
	httpClient      *http.Client
	kubeClient      *http.Client
	mu              sync.RWMutex
	agents          []AgentInfo
	lastError       string
}

func main() {
	srv, err := newServer()
	if err != nil {
		log.Fatalf("server init failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.refresh(ctx)
	go srv.refreshLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/agents", srv.handleAgents)
	mux.HandleFunc("/api/refresh", srv.handleRefresh)

	addr := ":80"
	log.Printf("k8s-tool-server listening on %s", addr)
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

	caPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	kubeHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubePort := env("KUBERNETES_SERVICE_PORT", "443")
	kubeAPI := ""
	if kubeHost != "" {
		kubeAPI = "https://" + netJoinHostPort(kubeHost, kubePort)
	}

	kubeClient := &http.Client{Timeout: 5 * time.Second}
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
		caPath:          caPath,
		httpClient:      &http.Client{Timeout: 3 * time.Second},
		kubeClient:      kubeClient,
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
	agents, err := s.fetchAgents(ctx)
	errText := ""
	if err != nil {
		errText = err.Error()
		log.Printf("refresh failed: %v", err)
	}
	if agents == nil {
		agents = []AgentInfo{}
	}
	s.mu.Lock()
	s.agents = agents
	s.lastError = errText
	s.mu.Unlock()
}

func (s *server) fetchAgents(ctx context.Context) ([]AgentInfo, error) {
	pods, err := s.listAgentPods(ctx)
	if err != nil {
		return nil, err
	}

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

	filtered := make([]pod, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Status.PodIP != "" && item.Status.Phase == "Running" {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *server) fetchAgentInfo(ctx context.Context, pod pod) AgentInfo {
	now := time.Now().Format(time.RFC3339)
	info := AgentInfo{
		PodName:       pod.Metadata.Name,
		Namespace:     pod.Metadata.Namespace,
		PodIP:         pod.Status.PodIP,
		Status:        "error",
		LastRefreshAt: now,
	}

	endpoint := fmt.Sprintf("http://%s:%d/api/node-info", pod.Status.PodIP, s.agentPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		info.Error = fmt.Sprintf("agent returned %s: %s", resp.Status, string(body))
		return info
	}
	if err := json.Unmarshal(body, &info); err != nil {
		info.Error = err.Error()
		return info
	}
	info.Status = "online"
	info.LastRefreshAt = now
	return info
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	data := struct {
		Agents          []AgentInfo
		LastError       string
		RefreshInterval int
		GeneratedAt     string
	}{
		Agents:          cloneAgents(s.agents),
		LastError:       s.lastError,
		RefreshInterval: int(s.refreshInterval.Seconds()),
		GeneratedAt:     time.Now().Format(time.RFC3339),
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, data); err != nil {
		log.Printf("render index failed: %v", err)
	}
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func cloneAgents(agents []AgentInfo) []AgentInfo {
	out := make([]AgentInfo, len(agents))
	copy(out, agents)
	return out
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

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="refresh" content="{{.RefreshInterval}}">
  <title>k8s-tool-server</title>
  <style>
    :root { color-scheme: light dark; font-family: Arial, Helvetica, sans-serif; background: #f6f8fb; color: #17202a; }
    body { margin: 0; padding: 24px; }
    header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 18px; }
    h1 { margin: 0 0 6px; font-size: 26px; letter-spacing: 0; }
    .meta { color: #52606d; font-size: 14px; }
    button { border: 1px solid #8aa4c2; background: #ffffff; color: #17202a; border-radius: 6px; padding: 8px 12px; cursor: pointer; }
    table { width: 100%; border-collapse: collapse; background: #ffffff; border: 1px solid #d9e2ec; }
    th, td { text-align: left; padding: 10px; border-bottom: 1px solid #d9e2ec; font-size: 14px; vertical-align: top; }
    th { background: #eef3f8; font-weight: 700; }
    .status { font-weight: 700; }
    .online { color: #16794c; }
    .error { color: #b42318; }
    .empty, .alert { border: 1px solid #d9e2ec; background: #ffffff; padding: 14px; border-radius: 8px; }
    .alert { border-color: #f4b6b0; color: #9f1d14; margin-bottom: 14px; }
    @media (prefers-color-scheme: dark) {
      :root { background: #121820; color: #eef2f6; }
      table, .empty, .alert, button { background: #1c2633; border-color: #34445a; color: #eef2f6; }
      th { background: #263446; }
      td, th { border-color: #34445a; }
      .meta { color: #b7c4d3; }
    }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>k8s-tool-server</h1>
      <div class="meta">Agents: {{len .Agents}} | Generated: {{.GeneratedAt}} | Refresh: {{.RefreshInterval}}s</div>
    </div>
    <button type="button" onclick="refreshNow()">Refresh</button>
  </header>
  {{if .LastError}}<div class="alert">{{.LastError}}</div>{{end}}
  {{if .Agents}}
  <table>
    <thead>
      <tr>
        <th>Status</th>
        <th>Node</th>
        <th>Hostname</th>
        <th>Pod IP</th>
        <th>Node IP</th>
        <th>Memory</th>
        <th>Last Refresh</th>
      </tr>
    </thead>
    <tbody>
      {{range .Agents}}
      <tr>
        <td class="status {{.Status}}">{{.Status}}{{if .Error}}<br>{{.Error}}{{end}}</td>
        <td>{{.NodeName}}</td>
        <td>{{.Hostname}}</td>
        <td>{{.PodIP}}</td>
        <td>{{.NodeIP}}</td>
        <td>{{.MemoryUsedKB}} / {{.MemoryTotalKB}} KB<br>free {{.MemoryFreeKB}} KB</td>
        <td>{{.LastRefreshAt}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">No agent data available.</div>
  {{end}}
  <script>
    function refreshNow() {
      fetch('/api/refresh', { method: 'POST' }).then(function () {
        window.location.reload();
      });
    }
  </script>
</body>
</html>`))

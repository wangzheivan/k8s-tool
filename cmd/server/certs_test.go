package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunAgentCertStatusCheckScansAndClassifiesCertificates(t *testing.T) {
	root := t.TempDir()
	serverDir := filepath.Join(root, "server", "tls")
	agentDir := filepath.Join(root, "agent")
	if err := os.MkdirAll(filepath.Join(serverDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agentDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestCert(t, filepath.Join(serverDir, "valid.crt"), "server-valid", time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))
	writeTestCert(t, filepath.Join(serverDir, "nested", "soon.pem"), "server-soon", time.Now().Add(-time.Hour), time.Now().Add(5*24*time.Hour))
	writeTestCert(t, filepath.Join(agentDir, "expired.crt"), "agent-expired", time.Now().Add(-90*24*time.Hour), time.Now().Add(-24*time.Hour))
	writeTestCert(t, filepath.Join(agentDir, "nested", "ignored.crt"), "agent-nested", time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))
	if err := os.WriteFile(filepath.Join(agentDir, "bad.pem"), []byte("not a cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "client-key.pem"), []byte("not a cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RKE2_SERVER_TLS_DIR", serverDir)
	t.Setenv("RKE2_AGENT_DIR", agentDir)

	result := runAgentCertStatusCheck(AgentInfo{NodeName: "node-a", NodeIP: "192.168.1.10", PodName: "agent-a", PodIP: "10.42.0.10"}, 30)

	if result.Status != certStatusParseError {
		t.Fatalf("status = %q, want parse-error; result=%+v", result.Status, result)
	}
	if result.ServerCertCount != 2 {
		t.Fatalf("ServerCertCount = %d, want 2", result.ServerCertCount)
	}
	if result.AgentCertCount != 2 {
		t.Fatalf("AgentCertCount = %d, want 2", result.AgentCertCount)
	}
	if result.ExpiredCount != 1 || result.ExpiringSoonCount != 1 || result.ParseErrorCount != 1 {
		t.Fatalf("unexpected counts expired=%d expiring=%d parse=%d", result.ExpiredCount, result.ExpiringSoonCount, result.ParseErrorCount)
	}
	for _, cert := range result.Certificates {
		if strings.Contains(cert.Path, "client-key") || strings.Contains(cert.Path, "nested/ignored") {
			t.Fatalf("unexpected certificate path included: %s", cert.Path)
		}
	}
	if len(result.Certificates) != 4 {
		t.Fatalf("len(Certificates) = %d, want 4", len(result.Certificates))
	}
	if result.Certificates[0].NotAfter > result.Certificates[1].NotAfter {
		t.Fatalf("certificates are not sorted by expiry: %#v", result.Certificates)
	}
}

func TestRunAgentCertStatusCheckMissingRKE2Path(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RKE2_SERVER_TLS_DIR", filepath.Join(root, "missing-server"))
	t.Setenv("RKE2_AGENT_DIR", filepath.Join(root, "missing-agent"))

	result := runAgentCertStatusCheck(AgentInfo{NodeName: "node-a"}, 30)

	if result.Status != certStatusMissingRKE2Path {
		t.Fatalf("status = %q, want missing-rke2-path", result.Status)
	}
	if len(result.Certificates) != 0 {
		t.Fatalf("len(Certificates) = %d, want 0", len(result.Certificates))
	}
}

func TestCertNodeRoleAndSummary(t *testing.T) {
	serverNode := CertNodeResult{NodeName: "server-a", Role: "server", Status: certStatusOK, Certificates: []CertificateInfo{{Category: "server", Status: certStatusExpired, Expired: true}}}
	workerNode := CertNodeResult{NodeName: "worker-a", Role: "worker", Status: certStatusOK, Certificates: []CertificateInfo{{Category: "agent", Status: certStatusExpiringSoon, ExpiringSoon: true}}}
	failedNode := CertNodeResult{NodeName: "worker-b", Role: "worker", Status: certStatusAgentConnectError}
	summary := CertStatusSummary{Results: []CertNodeResult{workerNode, failedNode, serverNode}}

	finalizeCertSummary(&summary)

	if summary.ServerNodeCount != 1 || summary.WorkerNodeCount != 2 || summary.CheckedNodeCount != 2 {
		t.Fatalf("unexpected node counts: %+v", summary)
	}
	if summary.TotalCertCount != 2 || summary.ExpiredCount != 1 || summary.ExpiringSoonCount != 1 {
		t.Fatalf("unexpected cert counts: %+v", summary)
	}
	if summary.Results[0].NodeName != "server-a" {
		t.Fatalf("results not sorted by node name: %#v", summary.Results)
	}
	if certNodeRole(node{Metadata: metadata{Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}}}) != "server" {
		t.Fatalf("control-plane node should be server")
	}
	if certNodeRole(node{Metadata: metadata{Labels: map[string]string{}}}) != "worker" {
		t.Fatalf("unlabeled node should be worker")
	}
}

func writeTestCert(t *testing.T, path, commonName string, notBefore, notAfter time.Time) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		Issuer:       pkix.Name{CommonName: commonName + "-issuer"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
}

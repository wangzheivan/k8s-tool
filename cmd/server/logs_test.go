package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidLogDays(t *testing.T) {
	for _, days := range []int{1, 3, 7, 14} {
		if !validLogDays(days) {
			t.Fatalf("validLogDays(%d) = false", days)
		}
	}
	for _, days := range []int{0, 2, 15} {
		if validLogDays(days) {
			t.Fatalf("validLogDays(%d) = true", days)
		}
	}
}

func TestFinalizeLogSummaryAggregatesResults(t *testing.T) {
	summary := LogCollectionSummary{
		CollectionID:  "collection-a",
		DownloadReady: true,
		Results: []LogNodeResult{
			{NodeName: "node-b", Status: logStatusCollectorError, Message: "failed"},
			{NodeName: "node-a", Status: logStatusOK, SizeBytes: 10},
			{NodeName: "node-c", Status: logStatusOK, SizeBytes: 20},
		},
	}

	finalizeLogSummary(&summary)

	if summary.CompletedNodeCount != 2 || summary.FailedNodeCount != 1 || summary.TotalBytes != 30 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Results[0].NodeName != "node-a" {
		t.Fatalf("results not sorted: %+v", summary.Results)
	}
	if summary.DownloadURL != "/api/logs/download/collection-a" {
		t.Fatalf("DownloadURL = %q", summary.DownloadURL)
	}
}

func TestSafeLogIDRejectsTraversal(t *testing.T) {
	if !safeLogID("20260606T010203Z") {
		t.Fatalf("safe id rejected")
	}
	for _, id := range []string{"", "../x", "a/b", "a b"} {
		if safeLogID(id) {
			t.Fatalf("unsafe id accepted: %q", id)
		}
	}
}

func TestRunAgentLogCollectorUsesFakeScript(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "collector.sh")
	if err := os.WriteFile(script, []byte(`#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -d) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
mkdir -p "$out"
printf 'fake logs' > "$out/fake-node.tar.gz"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOG_COLLECTION_DIR", filepath.Join(root, "collections"))
	t.Setenv("LOG_COLLECTOR_SCRIPT", script)
	t.Setenv("LOG_COLLECTOR_FORCE_POD_DISTRO", "true")

	result := runAgentLogCollector(context.Background(), AgentInfo{NodeName: "node-a", NodeIP: "192.168.1.10", PodName: "agent-a", PodIP: "10.42.0.10"}, 1)

	if result.Status != logStatusOK {
		t.Fatalf("status = %q message=%q", result.Status, result.Message)
	}
	if result.ArtifactID == "" || result.ArtifactName != "fake-node.tar.gz" || result.SizeBytes == 0 {
		t.Fatalf("unexpected artifact result: %+v", result)
	}
	if _, err := agentArtifactPath(result.ArtifactID); err != nil {
		t.Fatalf("artifact path not found: %v", err)
	}
}

func TestRunAgentLogCollectorRejectsInvalidDays(t *testing.T) {
	result := runAgentLogCollector(context.Background(), AgentInfo{NodeName: "node-a"}, 2)

	if result.Status != logStatusCollectorError {
		t.Fatalf("status = %q", result.Status)
	}
}

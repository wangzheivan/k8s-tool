package main

import "testing"

func TestBuildNetworkViewAggregatesResults(t *testing.T) {
	summary := NetworkCheckSummary{
		AgentCount: 2,
		Results: []NetworkCheckResult{
			{SourcePod: "agent-a", TargetPod: "agent-a", PingOK: true, HTTPOK: true},
			{SourcePod: "agent-a", TargetPod: "agent-b", PingOK: true, HTTPOK: false, HTTPError: "connection refused"},
			{SourcePod: "agent-b", TargetPod: "agent-a", Skipped: true, SkipReason: "source pod phase is Pending"},
		},
	}

	view := buildNetworkView(summary)

	if view.Stats.AgentCount != 2 {
		t.Fatalf("AgentCount = %d, want 2", view.Stats.AgentCount)
	}
	if view.Stats.TotalChecks != 3 {
		t.Fatalf("TotalChecks = %d, want 3", view.Stats.TotalChecks)
	}
	if view.Stats.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", view.Stats.SuccessCount)
	}
	if view.Stats.FailedCount != 2 {
		t.Fatalf("FailedCount = %d, want 2", view.Stats.FailedCount)
	}
	if view.Stats.SkippedCount != 1 {
		t.Fatalf("SkippedCount = %d, want 1", view.Stats.SkippedCount)
	}
	if view.Stats.HTTPFailed != 1 {
		t.Fatalf("HTTPFailed = %d, want 1", view.Stats.HTTPFailed)
	}
	if len(view.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2", len(view.Sources))
	}
	if view.Sources[0].SourcePod != "agent-a" || view.Sources[0].FailedCount != 1 || len(view.Sources[0].Failures) != 1 {
		t.Fatalf("unexpected first source summary: %+v", view.Sources[0])
	}
	if len(view.Failures) != 2 {
		t.Fatalf("len(Failures) = %d, want 2", len(view.Failures))
	}
}

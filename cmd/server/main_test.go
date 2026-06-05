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

func TestBuildNetworkViewTreatsPingOnlyLayersAsSuccess(t *testing.T) {
	summary := NetworkCheckSummary{
		AgentCount: 2,
		Results: []NetworkCheckResult{
			{Layer: "node-to-node", SourcePod: "agent-a", TargetName: "node-a", PingOK: true},
			{Layer: "pod-to-node", SourcePod: "agent-a", TargetName: "node-b", PingOK: true},
			{Layer: "node-to-pod", SourcePod: "agent-a", TargetName: "agent-b", PingOK: true, HTTPOK: false, HTTPError: "timeout"},
		},
	}

	view := buildNetworkView(summary)

	if view.Stats.SuccessCount != 2 {
		t.Fatalf("SuccessCount = %d, want 2", view.Stats.SuccessCount)
	}
	if view.Stats.FailedCount != 1 {
		t.Fatalf("FailedCount = %d, want 1", view.Stats.FailedCount)
	}
	if view.Stats.HTTPFailed != 1 {
		t.Fatalf("HTTPFailed = %d, want 1", view.Stats.HTTPFailed)
	}
}

func TestAgentPodsByNodePrefersRunningCheckableAgent(t *testing.T) {
	pods := []pod{
		{Metadata: metadata{Name: "agent-pending"}, Spec: podSpec{NodeName: "node-a"}, Status: podStatus{Phase: "Pending"}},
		{Metadata: metadata{Name: "agent-running"}, Spec: podSpec{NodeName: "node-a"}, Status: podStatus{Phase: "Running", PodIP: "10.42.0.10"}},
		{Metadata: metadata{Name: "agent-b"}, Spec: podSpec{NodeName: "node-b"}, Status: podStatus{Phase: "Running", PodIP: "10.42.0.11"}},
	}

	byNode := agentPodsByNode(pods)

	if byNode["node-a"].Metadata.Name != "agent-running" {
		t.Fatalf("node-a agent = %s, want agent-running", byNode["node-a"].Metadata.Name)
	}
	if byNode["node-b"].Metadata.Name != "agent-b" {
		t.Fatalf("node-b agent = %s, want agent-b", byNode["node-b"].Metadata.Name)
	}
}

func TestEtcdNodeResultUsesInternalIP(t *testing.T) {
	n := node{
		Metadata: metadata{Name: "node-a"},
		Status: nodeStatus{Addresses: []nodeAddress{
			{Type: "Hostname", Address: "node-a.local"},
			{Type: "InternalIP", Address: "192.168.50.10"},
		}},
	}

	result := etcdNodeResult(n, "missing-agent-on-etcd-node", "missing")

	if result.NodeIP != "192.168.50.10" {
		t.Fatalf("NodeIP = %q, want 192.168.50.10", result.NodeIP)
	}
	if result.Status != "missing-agent-on-etcd-node" {
		t.Fatalf("Status = %q, want missing-agent-on-etcd-node", result.Status)
	}
}

func TestEndpointsFromMemberListJSON(t *testing.T) {
	raw := `{"members":[{"clientURLs":["https://10.0.0.1:2379"]},{"clientURLs":["https://10.0.0.2:2379","https://10.0.0.2:2379"]}]}`

	endpoints := endpointsFromMemberList(raw)

	if len(endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2: %#v", len(endpoints), endpoints)
	}
	if endpoints[0] != "https://10.0.0.1:2379" || endpoints[1] != "https://10.0.0.2:2379" {
		t.Fatalf("unexpected endpoints: %#v", endpoints)
	}
}

func TestEndpointsFromMemberListTextFallback(t *testing.T) {
	raw := "1c424074df86e854, started, node-a, https://10.0.0.1:2380, https://10.0.0.1:2379, false\n45c68c44c5a792ff, started, node-b, https://10.0.0.2:2380, https://10.0.0.2:2379, false"

	endpoints := endpointsFromMemberList(raw)

	if len(endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2: %#v", len(endpoints), endpoints)
	}
	if endpoints[0] != "https://10.0.0.1:2379" || endpoints[1] != "https://10.0.0.2:2379" {
		t.Fatalf("unexpected endpoints: %#v", endpoints)
	}
}

func TestCountEtcdAlarms(t *testing.T) {
	commands := []EtcdCommandResult{
		{Name: "member-list", ExitCode: 0, Stdout: "ignored"},
		{Name: "alarm-list", ExitCode: 0, Stdout: "memberID:1 alarm:NOSPACE\nmemberID:2 alarm:NOSPACE\n"},
	}

	count := countEtcdAlarms(commands)

	if count != 2 {
		t.Fatalf("alarm count = %d, want 2", count)
	}
}

func TestBuildHostNetworkCrictlCommand(t *testing.T) {
	display, name, args := buildHostNetworkCrictlCommand("/var/lib/rancher/rke2/bin/crictl", "exec", "container-a", "etcdctl", "member", "list")

	if name != "nsenter" {
		t.Fatalf("command name = %q, want nsenter", name)
	}
	wantArgs := []string{"-t", "1", "-n", "/var/lib/rancher/rke2/bin/crictl", "exec", "container-a", "etcdctl", "member", "list"}
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Fatalf("args[%d] = %q, want %q; all args=%#v", i, args[i], wantArgs[i], args)
		}
	}
	if display != "nsenter -t 1 -n /var/lib/rancher/rke2/bin/crictl exec container-a etcdctl member list" {
		t.Fatalf("display = %q", display)
	}
}

func TestFinalizeEtcdSummary(t *testing.T) {
	summary := EtcdStatusSummary{
		EtcdNodeCount: 2,
		Results: []EtcdStatusResult{
			{Status: "ok", AlarmCount: 1},
			{Status: "missing-agent-on-etcd-node"},
		},
	}

	finalizeEtcdSummary(&summary)

	if summary.CheckedNodeCount != 2 || summary.HealthyNodeCount != 1 || summary.UnhealthyNodeCount != 1 || summary.AlarmCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

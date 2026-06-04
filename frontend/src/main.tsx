import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { fetchAgents, fetchNetworkCheck, refreshAgents, runNetworkCheck } from "./api";
import { AgentStatus } from "./components/AgentStatus";
import { NetworkDiagnostics } from "./components/NetworkDiagnostics";
import "./styles.css";
import type { AgentInfo, AgentsResponse, NetworkCheckSummary } from "./types";

function App() {
  const [agentsResponse, setAgentsResponse] = useState<AgentsResponse>({ agents: [] });
  const [network, setNetwork] = useState<NetworkCheckSummary>({ running: false, agentCount: 0, results: [] });
  const [loadingAgents, setLoadingAgents] = useState(false);
  const [runningNetwork, setRunningNetwork] = useState(false);
  const [error, setError] = useState("");
  const generatedAt = useMemo(() => new Date().toISOString(), [agentsResponse, network]);

  async function loadAgents() {
    setLoadingAgents(true);
    setError("");
    try {
      setAgentsResponse(await fetchAgents());
    } catch (err) {
      setError(String(err));
    } finally {
      setLoadingAgents(false);
    }
  }

  async function loadNetwork() {
    try {
      setNetwork(await fetchNetworkCheck());
    } catch (err) {
      setError(String(err));
    }
  }

  async function handleRefreshAgents() {
    setLoadingAgents(true);
    setError("");
    try {
      setAgentsResponse(await refreshAgents());
    } catch (err) {
      setError(String(err));
    } finally {
      setLoadingAgents(false);
    }
  }

  async function handleRunNetworkCheck() {
    setRunningNetwork(true);
    setError("");
    try {
      setNetwork(await runNetworkCheck());
    } catch (err) {
      setError(String(err));
    } finally {
      setRunningNetwork(false);
    }
  }

  useEffect(() => {
    loadAgents();
    loadNetwork();
    const id = window.setInterval(loadAgents, 10_000);
    return () => window.clearInterval(id);
  }, []);

  const agents: AgentInfo[] = agentsResponse.agents ?? [];

  return (
    <main>
      <header>
        <div>
          <h1>k8s-tool-server</h1>
          <div className="meta">
            Agents: {agents.length} | Generated: {generatedAt} | Refresh: 10s
          </div>
        </div>
        <button type="button" onClick={handleRefreshAgents} disabled={loadingAgents}>
          {loadingAgents ? "Refreshing..." : "Refresh Agents"}
        </button>
      </header>
      {(error || agentsResponse.lastError) && <div className="alert">{error || agentsResponse.lastError}</div>}
      <AgentStatus agents={agents} />
      <NetworkDiagnostics network={network} running={runningNetwork} onRun={handleRunNetworkCheck} />
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

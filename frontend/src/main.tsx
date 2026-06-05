import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { fetchAgents, fetchCertStatus, fetchEtcdStatus, fetchLayeredNetworkCheck, refreshAgents, runCertStatus, runEtcdStatus, runLayeredNetworkCheck } from "./api";
import { AgentStatus } from "./components/AgentStatus";
import { CertificateStatus } from "./components/CertificateStatus";
import { EtcdStatus } from "./components/EtcdStatus";
import { NetworkDiagnostics } from "./components/NetworkDiagnostics";
import "./styles.css";
import type { AgentInfo, AgentsResponse, CertStatusSummary, EtcdStatusSummary, NetworkCheckSummary } from "./types";

function App() {
  const [agentsResponse, setAgentsResponse] = useState<AgentsResponse>({ agents: [] });
  const [network, setNetwork] = useState<NetworkCheckSummary>({ running: false, agentCount: 0, results: [] });
  const [etcd, setEtcd] = useState<EtcdStatusSummary>({ running: false, etcdNodeCount: 0, checkedNodeCount: 0, healthyNodeCount: 0, unhealthyNodeCount: 0, alarmCount: 0, results: [] });
  const [certs, setCerts] = useState<CertStatusSummary>({ running: false, nodeCount: 0, serverNodeCount: 0, workerNodeCount: 0, checkedNodeCount: 0, totalCertCount: 0, expiredCount: 0, expiringSoonCount: 0, parseErrorCount: 0, results: [] });
  const [loadingAgents, setLoadingAgents] = useState(false);
  const [runningNetwork, setRunningNetwork] = useState(false);
  const [runningEtcd, setRunningEtcd] = useState(false);
  const [runningCerts, setRunningCerts] = useState(false);
  const [error, setError] = useState("");
  const generatedAt = useMemo(() => new Date().toISOString(), [agentsResponse, network, etcd]);

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
      setNetwork(await fetchLayeredNetworkCheck());
    } catch (err) {
      setError(String(err));
    }
  }

  async function loadEtcd() {
    try {
      setEtcd(await fetchEtcdStatus());
    } catch (err) {
      setError(String(err));
    }
  }

  async function loadCerts() {
    try {
      setCerts(await fetchCertStatus());
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
      setNetwork(await runLayeredNetworkCheck());
    } catch (err) {
      setError(String(err));
    } finally {
      setRunningNetwork(false);
    }
  }

  async function handleRunEtcdStatus() {
    setRunningEtcd(true);
    setError("");
    try {
      setEtcd(await runEtcdStatus());
    } catch (err) {
      setError(String(err));
    } finally {
      setRunningEtcd(false);
    }
  }

  async function handleRunCertStatus() {
    setRunningCerts(true);
    setError("");
    try {
      setCerts(await runCertStatus());
    } catch (err) {
      setError(String(err));
    } finally {
      setRunningCerts(false);
    }
  }

  useEffect(() => {
    loadAgents();
    loadNetwork();
    loadEtcd();
    loadCerts();
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
      <EtcdStatus etcd={etcd} running={runningEtcd} onRun={handleRunEtcdStatus} />
      <CertificateStatus certs={certs} running={runningCerts} onRun={handleRunCertStatus} />
    </main>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { fetchAgents, fetchCertStatus, fetchEtcdStatus, fetchLayeredNetworkCheck, refreshAgents, runCertStatus, runEtcdStatus, runLayeredNetworkCheck } from "./api";
import { AgentStatus } from "./components/AgentStatus";
import { CertificateStatus } from "./components/CertificateStatus";
import { EtcdStatus } from "./components/EtcdStatus";
import { NetworkDiagnostics } from "./components/NetworkDiagnostics";
import "./styles.css";
import type { AgentInfo, AgentsResponse, CertStatusSummary, EtcdStatusSummary, NetworkCheckSummary } from "./types";

type ViewID = "agents" | "network" | "etcd" | "certs";

const views: Array<{ id: ViewID; label: string; description: string }> = [
  { id: "agents", label: "Agent Status", description: "Node agent discovery and health" },
  { id: "network", label: "Network Diagnostics", description: "Layered pod and node connectivity checks" },
  { id: "etcd", label: "Etcd Status", description: "RKE2 etcd read-only status checks" },
  { id: "certs", label: "Certificate Status", description: "RKE2 certificate expiry and parse checks" },
];

function App() {
  const [activeView, setActiveView] = useState<ViewID>("agents");
  const [agentsResponse, setAgentsResponse] = useState<AgentsResponse>({ agents: [] });
  const [network, setNetwork] = useState<NetworkCheckSummary>({ running: false, agentCount: 0, results: [] });
  const [etcd, setEtcd] = useState<EtcdStatusSummary>({ running: false, etcdNodeCount: 0, checkedNodeCount: 0, healthyNodeCount: 0, unhealthyNodeCount: 0, alarmCount: 0, results: [] });
  const [certs, setCerts] = useState<CertStatusSummary>({ running: false, nodeCount: 0, serverNodeCount: 0, workerNodeCount: 0, checkedNodeCount: 0, totalCertCount: 0, expiredCount: 0, expiringSoonCount: 0, parseErrorCount: 0, results: [] });
  const [loadingAgents, setLoadingAgents] = useState(false);
  const [runningNetwork, setRunningNetwork] = useState(false);
  const [runningEtcd, setRunningEtcd] = useState(false);
  const [runningCerts, setRunningCerts] = useState(false);
  const [error, setError] = useState("");
  const generatedAt = useMemo(() => new Date().toISOString(), [agentsResponse, network, etcd, certs]);

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
  const activeMeta = views.find((view) => view.id === activeView) ?? views[0];

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="Feature navigation">
        <div className="brand">
          <h1>k8s-tool</h1>
          <div className="meta">Troubleshooting Console</div>
        </div>
        <nav className="module-nav">
          {views.map((view) => (
            <button type="button" className={`nav-button ${activeView === view.id ? "active" : ""}`} onClick={() => setActiveView(view.id)} key={view.id}>
              <span>{view.label}</span>
              <small>{view.description}</small>
            </button>
          ))}
        </nav>
      </aside>
      <main className="content">
        <header>
          <div>
            <h1>{activeMeta.label}</h1>
            <div className="meta">
              k8s-tool-server | Agents: {agents.length} | Generated: {generatedAt} | Refresh: 10s
            </div>
          </div>
          <button type="button" onClick={handleRefreshAgents} disabled={loadingAgents}>
            {loadingAgents ? "Refreshing..." : "Refresh Agents"}
          </button>
        </header>
        {(error || agentsResponse.lastError) && <div className="alert">{error || agentsResponse.lastError}</div>}
        <div className="page-panel">
          {activeView === "agents" && <AgentStatus agents={agents} />}
          {activeView === "network" && <NetworkDiagnostics network={network} running={runningNetwork} onRun={handleRunNetworkCheck} />}
          {activeView === "etcd" && <EtcdStatus etcd={etcd} running={runningEtcd} onRun={handleRunEtcdStatus} />}
          {activeView === "certs" && <CertificateStatus certs={certs} running={runningCerts} onRun={handleRunCertStatus} />}
        </div>
      </main>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

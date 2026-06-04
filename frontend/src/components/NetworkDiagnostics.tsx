import { useState } from "react";
import type { NetworkCheckSummary } from "../types";
import { buildNetworkView } from "../networkView";
import { SummaryCards } from "./SummaryCards";
import { SourceSummaryTable } from "./SourceSummaryTable";
import { FailuresTable } from "./FailuresTable";

interface NetworkDiagnosticsProps {
  network: NetworkCheckSummary;
  running: boolean;
  onRun: () => void;
}

const layers = [
  { id: "pod-to-pod", label: "Pod to Pod" },
  { id: "node-to-node", label: "Node to Node" },
  { id: "node-to-pod", label: "Node to Pod" },
  { id: "pod-to-node", label: "Pod to Node" },
] as const;

export function NetworkDiagnostics({ network, running, onRun }: NetworkDiagnosticsProps) {
  const [activeTab, setActiveTab] = useState<string>("pod-to-pod");
  const allView = buildNetworkView(network);
  const activeLayer = layers.find((layer) => layer.id === activeTab);
  const view = activeLayer ? buildNetworkView(network, activeLayer.id) : allView;
  const resultCount = network.results?.length ?? 0;

  return (
    <section>
      <div className="section-head">
        <div>
          <h2>Network Diagnostics</h2>
          <div className="meta">
            Last run: {network.completedAt ?? ""} | Results: {resultCount}
          </div>
        </div>
        <button type="button" onClick={onRun} disabled={running}>
          {running ? "Running..." : "Run Layered Network Check"}
        </button>
      </div>
      {network.error && <div className="alert">{network.error}</div>}
      {resultCount ? (
        <>
          <div className="tabs" role="tablist" aria-label="Network diagnostic layers">
            {layers.map((layer) => (
              <button className={activeTab === layer.id ? "active" : ""} key={layer.id} type="button" onClick={() => setActiveTab(layer.id)}>
                {layer.label}
              </button>
            ))}
            <button className={activeTab === "failures" ? "active" : ""} type="button" onClick={() => setActiveTab("failures")}>
              Failures
            </button>
          </div>
          {activeTab === "failures" ? (
            <>
              <SummaryCards
                cards={[
                  { label: "Agents", value: allView.stats.agentCount },
                  { label: "Checks", value: allView.stats.totalChecks },
                  { label: "Success", value: allView.stats.successCount, tone: "ok" },
                  { label: "Failed", value: allView.stats.failedCount, tone: "failed" },
                  { label: "Skipped", value: allView.stats.skippedCount },
                  { label: "Ping Failed", value: allView.stats.pingFailed, tone: "failed" },
                  { label: "HTTP Failed", value: allView.stats.httpFailed, tone: "failed" },
                ]}
              />
              <h3>All Failures</h3>
              {allView.failures.length ? <FailuresTable failures={allView.failures} showLayer /> : <div className="empty">No network failures detected.</div>}
            </>
          ) : (
            <>
              <SummaryCards
                cards={[
                  { label: "Agents", value: view.stats.agentCount },
                  { label: "Checks", value: view.stats.totalChecks },
                  { label: "Success", value: view.stats.successCount, tone: "ok" },
                  { label: "Failed", value: view.stats.failedCount, tone: "failed" },
                  { label: "Skipped", value: view.stats.skippedCount },
                  { label: "Ping Failed", value: view.stats.pingFailed, tone: "failed" },
                  { label: "HTTP Failed", value: view.stats.httpFailed, tone: "failed" },
                ]}
              />
              <h3>{activeLayer?.label} By Source</h3>
              <SourceSummaryTable sources={view.sources} />
              <h3>{activeLayer?.label} Failures</h3>
              {view.failures.length ? <FailuresTable failures={view.failures} /> : <div className="empty">No failures detected for this layer.</div>}
            </>
          )}
        </>
      ) : (
        <div className="empty">No network check has been run.</div>
      )}
    </section>
  );
}

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

export function NetworkDiagnostics({ network, running, onRun }: NetworkDiagnosticsProps) {
  const view = buildNetworkView(network);

  return (
    <section>
      <div className="section-head">
        <div>
          <h2>Network Diagnostics</h2>
          <div className="meta">
            Last run: {network.completedAt ?? ""} | Results: {network.results?.length ?? 0}
          </div>
        </div>
        <button type="button" onClick={onRun} disabled={running}>
          {running ? "Running..." : "Run Network Check"}
        </button>
      </div>
      {network.error && <div className="alert">{network.error}</div>}
      {network.results?.length ? (
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
          <h3>By Source</h3>
          <SourceSummaryTable sources={view.sources} />
          <h3>Failures</h3>
          {view.failures.length ? <FailuresTable failures={view.failures} /> : <div className="empty">No network failures detected.</div>}
        </>
      ) : (
        <div className="empty">No network check has been run.</div>
      )}
    </section>
  );
}

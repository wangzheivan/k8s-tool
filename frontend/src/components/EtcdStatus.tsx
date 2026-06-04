import type { EtcdCommandResult, EtcdStatusSummary } from "../types";
import { SummaryCards } from "./SummaryCards";

interface EtcdStatusProps {
  etcd: EtcdStatusSummary;
  running: boolean;
  onRun: () => void;
}

export function EtcdStatus({ etcd, running, onRun }: EtcdStatusProps) {
  const results = etcd.results ?? [];

  return (
    <section>
      <div className="section-head">
        <div>
          <h2>Etcd Status</h2>
          <div className="meta">
            Last run: {etcd.completedAt ?? ""} | Etcd nodes: {etcd.etcdNodeCount ?? 0}
          </div>
        </div>
        <button type="button" onClick={onRun} disabled={running}>
          {running ? "Checking..." : "Run Etcd Check"}
        </button>
      </div>
      {etcd.error && <div className="alert">{etcd.error}</div>}
      <SummaryCards
        cards={[
          { label: "Etcd Nodes", value: etcd.etcdNodeCount ?? 0 },
          { label: "Checked", value: etcd.checkedNodeCount ?? 0 },
          { label: "Healthy", value: etcd.healthyNodeCount ?? 0, tone: "ok" },
          { label: "Unhealthy", value: etcd.unhealthyNodeCount ?? 0, tone: "failed" },
          { label: "Alarms", value: etcd.alarmCount ?? 0, tone: etcd.alarmCount ? "failed" : undefined },
        ]}
      />
      {results.length ? (
        <table>
          <thead>
            <tr>
              <th>Status</th>
              <th>Node</th>
              <th>Node IP</th>
              <th>Agent Pod</th>
              <th>Etcd Container</th>
              <th>Alarms</th>
              <th>Checked</th>
            </tr>
          </thead>
          <tbody>
            {results.map((result) => (
              <tr key={`${result.nodeName}-${result.agentPod ?? "missing"}`}>
                <td>
                  <span className={`status ${result.status}`}>{result.status}</span>
                  {result.message && <div className="meta">{result.message}</div>}
                </td>
                <td>{result.nodeName}</td>
                <td>{result.nodeIP}</td>
                <td>
                  {result.agentPod ?? ""}
                  {result.agentPodIP && <div className="meta">{result.agentPodIP}</div>}
                </td>
                <td>{result.etcdContainerID ? shortID(result.etcdContainerID) : ""}</td>
                <td>{result.alarmCount}</td>
                <td>
                  {result.checkedAt}
                  <div className="meta">{result.durationMS} ms</div>
                  <details>
                    <summary>Details</summary>
                    <div className="detail-grid">
                      <div>
                        <strong>Agent URL</strong>
                        <pre>{result.agentURL ?? ""}</pre>
                      </div>
                      <div>
                        <strong>Commands</strong>
                        {result.commands?.length ? result.commands.map((command) => <CommandBlock command={command} key={command.name} />) : <pre>No command output.</pre>}
                      </div>
                    </div>
                  </details>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <div className="empty">No etcd check has been run.</div>
      )}
    </section>
  );
}

function CommandBlock({ command }: { command: EtcdCommandResult }) {
  const ok = command.exitCode === 0;
  return (
    <details className="command-block">
      <summary>
        {command.name} <span className={ok ? "ok" : "failed"}>exit {command.exitCode}</span> <span className="meta">{command.durationMS} ms</span>
      </summary>
      <div className="meta">{command.command}</div>
      {command.error && (
        <>
          <strong>Error</strong>
          <pre>{command.error}</pre>
        </>
      )}
      {command.stderr && (
        <>
          <strong>Stderr</strong>
          <pre>{command.stderr}</pre>
        </>
      )}
      {command.stdout && (
        <>
          <strong>Stdout</strong>
          <pre>{command.stdout}</pre>
        </>
      )}
    </details>
  );
}

function shortID(id: string) {
  return id.length > 12 ? id.slice(0, 12) : id;
}

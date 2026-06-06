import { useState } from "react";
import type { LogCollectionSummary, LogNodeResult } from "../types";
import { SummaryCards } from "./SummaryCards";

interface LogsCollectorProps {
  logs: LogCollectionSummary;
  running: boolean;
  onRun: (days: number) => void;
}

const dayOptions = [
  { value: 1, label: "1 day" },
  { value: 3, label: "3 days" },
  { value: 7, label: "7 days" },
  { value: 14, label: "14 days" },
];

export function LogsCollector({ logs, running, onRun }: LogsCollectorProps) {
  const [days, setDays] = useState<number | "">("");
  const canRun = days !== "" && !running && !logs.running;

  return (
    <section>
      <div className="section-head">
        <div>
          <h2>Logs Collector</h2>
          <div className="meta">
            Last run: {logs.completedAt ?? ""} | Collection: {logs.collectionID ?? ""}
          </div>
        </div>
        <button type="button" onClick={() => days !== "" && onRun(days)} disabled={!canRun}>
          {running || logs.running ? "Collecting..." : "Collect Logs"}
        </button>
      </div>
      <div className="alert">
        Log bundles may include sensitive cluster, node, workload, and system information. Download and share them carefully.
      </div>
      <div className="controls">
        <label>
          Time range
          <select value={days} onChange={(event) => setDays(event.target.value ? Number(event.target.value) : "")}>
            <option value="">Select range</option>
            {dayOptions.map((option) => (
              <option value={option.value} key={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        {logs.downloadReady && logs.downloadURL && (
          <a className="button-link" href={logs.downloadURL}>
            Download Logs
          </a>
        )}
      </div>
      {logs.error && <div className="alert">{logs.error}</div>}
      <SummaryCards
        cards={[
          { label: "Nodes", value: logs.nodeCount ?? 0 },
          { label: "Completed", value: logs.completedNodeCount ?? 0, tone: logs.completedNodeCount ? "ok" : undefined },
          { label: "Failed", value: logs.failedNodeCount ?? 0, tone: logs.failedNodeCount ? "failed" : undefined },
          { label: "Total Size", value: formatBytes(logs.totalBytes ?? 0) },
          { label: "Days", value: logs.days ?? "" },
          { label: "Download", value: logs.downloadReady ? "ready" : "not ready", tone: logs.downloadReady ? "ok" : undefined },
        ]}
      />
      {logs.results?.length ? <LogNodeTable results={logs.results} /> : <div className="empty">No log collection has been run.</div>}
    </section>
  );
}

function LogNodeTable({ results }: { results: LogNodeResult[] }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Status</th>
          <th>Node</th>
          <th>Agent Pod</th>
          <th>Artifact</th>
          <th>Size</th>
          <th>Duration</th>
          <th>Completed</th>
        </tr>
      </thead>
      <tbody>
        {results.map((result) => (
          <tr key={`${result.nodeName}-${result.agentPod ?? "missing"}`}>
            <td>
              <span className={`status ${result.status}`}>{result.status}</span>
              {result.message && (
                <details>
                  <summary>Message</summary>
                  <pre>{result.message}</pre>
                </details>
              )}
            </td>
            <td>
              {result.nodeName}
              <div className="meta">{result.nodeIP}</div>
            </td>
            <td>
              {result.agentPod ?? ""}
              {result.agentPodIP && <div className="meta">{result.agentPodIP}</div>}
            </td>
            <td>
              <code>{result.artifactName ?? ""}</code>
            </td>
            <td>{formatBytes(result.sizeBytes ?? 0)}</td>
            <td>{result.durationMS ? `${result.durationMS} ms` : ""}</td>
            <td>{result.completedAt ?? ""}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function formatBytes(value: number) {
  if (!value) {
    return "0 B";
  }
  const units = ["B", "KiB", "MiB", "GiB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

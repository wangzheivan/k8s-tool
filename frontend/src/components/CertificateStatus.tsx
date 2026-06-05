import { useState } from "react";
import type { CertNodeResult, CertStatusSummary, CertificateInfo } from "../types";
import { SummaryCards } from "./SummaryCards";

interface CertificateStatusProps {
  certs: CertStatusSummary;
  running: boolean;
  onRun: () => void;
}

const filters = [
  { id: "all", label: "All" },
  { id: "expired", label: "Expired" },
  { id: "expiring", label: "Expiring Soon" },
  { id: "parse", label: "Parse Errors" },
];

export function CertificateStatus({ certs, running, onRun }: CertificateStatusProps) {
  const [filter, setFilter] = useState("all");
  const results = (certs.results ?? [])
    .map((result) => ({ ...result, certificates: filterCertificates(result.certificates ?? [], filter) }))
    .filter((result) => filter === "all" || result.certificates.length > 0 || nodeMatchesFilter(result, filter));

  return (
    <section>
      <div className="section-head">
        <div>
          <h2>Certificate Status</h2>
          <div className="meta">
            Last run: {certs.completedAt ?? ""} | Nodes: {certs.nodeCount ?? 0}
          </div>
        </div>
        <button type="button" onClick={onRun} disabled={running}>
          {running ? "Checking..." : "Run Certificate Check"}
        </button>
      </div>
      {certs.error && <div className="alert">{certs.error}</div>}
      <SummaryCards
        cards={[
          { label: "Certificates", value: certs.totalCertCount ?? 0 },
          { label: "Expired", value: certs.expiredCount ?? 0, tone: certs.expiredCount ? "failed" : undefined },
          { label: "Expiring Soon", value: certs.expiringSoonCount ?? 0, tone: certs.expiringSoonCount ? "failed" : undefined },
          { label: "Parse Errors", value: certs.parseErrorCount ?? 0, tone: certs.parseErrorCount ? "failed" : undefined },
          { label: "Server Nodes", value: certs.serverNodeCount ?? 0 },
          { label: "Worker Nodes", value: certs.workerNodeCount ?? 0 },
        ]}
      />
      <div className="tabs" role="tablist" aria-label="Certificate status filters">
        {filters.map((item) => (
          <button type="button" className={filter === item.id ? "active" : ""} onClick={() => setFilter(item.id)} key={item.id}>
            {item.label}
          </button>
        ))}
      </div>
      {results.length ? <CertificateNodeTable results={results} /> : <div className="empty">No certificate check has been run.</div>}
    </section>
  );
}

function CertificateNodeTable({ results }: { results: CertNodeResult[] }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Status</th>
          <th>Node</th>
          <th>Role</th>
          <th>Agent Pod</th>
          <th>Server Certs</th>
          <th>Agent Certs</th>
          <th>Expired</th>
          <th>Expiring Soon</th>
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
            <td>
              {result.nodeName}
              <div className="meta">{result.nodeIP}</div>
            </td>
            <td>{result.role}</td>
            <td>
              {result.agentPod ?? ""}
              {result.agentPodIP && <div className="meta">{result.agentPodIP}</div>}
            </td>
            <td>{result.serverCertCount}</td>
            <td>{result.agentCertCount}</td>
            <td>{result.expiredCount}</td>
            <td>{result.expiringSoonCount}</td>
            <td>
              {result.checkedAt}
              <div className="meta">{result.durationMS} ms</div>
              <details>
                <summary>Certificates ({result.certificates?.length ?? 0})</summary>
                <CertificateDetailTable certificates={result.certificates ?? []} />
              </details>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function CertificateDetailTable({ certificates }: { certificates: CertificateInfo[] }) {
  if (!certificates.length) {
    return <div className="empty compact">No certificates for this filter.</div>;
  }
  return (
    <table className="nested-table">
      <thead>
        <tr>
          <th>Status</th>
          <th>Category</th>
          <th>Start Time</th>
          <th>Expire Time</th>
          <th>Days Left</th>
          <th>Subject</th>
          <th>SSL Path Name</th>
        </tr>
      </thead>
      <tbody>
        {certificates.map((cert) => (
          <tr key={`${cert.category}-${cert.path}`}>
            <td>
              <span className={`status ${cert.status}`}>{cert.status}</span>
              {cert.parseError && <div className="meta">{cert.parseError}</div>}
            </td>
            <td>{cert.category}</td>
            <td>{shortDate(cert.notBefore)}</td>
            <td>{shortDate(cert.notAfter)}</td>
            <td>{cert.daysLeft}</td>
            <td>{cert.subject}</td>
            <td>
              <code>{cert.path}</code>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function filterCertificates(certs: CertificateInfo[], filter: string) {
  switch (filter) {
    case "expired":
      return certs.filter((cert) => cert.expired);
    case "expiring":
      return certs.filter((cert) => cert.expiringSoon);
    case "parse":
      return certs.filter((cert) => cert.parseError);
    default:
      return certs;
  }
}

function nodeMatchesFilter(result: CertNodeResult, filter: string) {
  if (filter === "expired") {
    return result.status === "expired";
  }
  if (filter === "expiring") {
    return result.status === "expiring-soon";
  }
  if (filter === "parse") {
    return result.status === "parse-error";
  }
  return true;
}

function shortDate(value: string) {
  return value ? value.slice(0, 10) : "";
}

import type { NetworkCheckResult } from "../types";

interface FailuresTableProps {
  failures: NetworkCheckResult[];
  showLayer?: boolean;
}

export function FailuresTable({ failures, showLayer = false }: FailuresTableProps) {
  return (
    <table>
      <thead>
        <tr>
          {showLayer && <th>Layer</th>}
          <th>Source Pod</th>
          <th>Target</th>
          <th>Target IP</th>
          <th>Ping</th>
          <th>HTTP</th>
          <th>Checked</th>
        </tr>
      </thead>
      <tbody>
        {failures.map((failure) => (
          <tr key={`${failure.layer}-${failure.sourcePod}-${failure.targetName}-${failure.targetIP}`}>
            {showLayer && <td>{failure.layer}</td>}
            <td>
              {failure.sourcePod}
              <br />
              <span className="meta">
                {failure.sourceNode} {failure.sourceIP}
              </span>
            </td>
            <td>
              {failure.targetName || failure.targetPod}
              <br />
              <span className="meta">{failure.targetNode}</span>
            </td>
            <td>{failure.targetIP}</td>
            <td className={failure.pingOK ? "ok" : "failed"}>
              {failure.skipped ? "skipped" : `${failure.pingOK ? "ok" : "failed"} ${failure.pingDurationMS}ms`}
              <details>
                <summary>Details</summary>
                <pre>{`${failure.pingError ?? ""}${failure.skipReason ?? ""}`}</pre>
              </details>
            </td>
            <td className={failure.httpOK ? "ok" : "failed"}>
              {failure.skipped ? "skipped" : failure.httpDurationMS ? `${failure.httpOK ? "ok" : "failed"} status=${failure.httpStatus ?? 0} ${failure.httpDurationMS}ms` : "not checked"}
              <details>
                <summary>Details</summary>
                <pre>{`${failure.httpError ?? ""}${failure.skipReason ?? ""}`}</pre>
              </details>
            </td>
            <td>{failure.checkedAt}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

import type { NetworkCheckResult } from "../types";

interface FailuresTableProps {
  failures: NetworkCheckResult[];
}

export function FailuresTable({ failures }: FailuresTableProps) {
  return (
    <table>
      <thead>
        <tr>
          <th>Source Pod</th>
          <th>Target Pod</th>
          <th>Target IP</th>
          <th>Ping</th>
          <th>HTTP</th>
          <th>Checked</th>
        </tr>
      </thead>
      <tbody>
        {failures.map((failure) => (
          <tr key={`${failure.sourcePod}-${failure.targetPod}-${failure.targetIP}`}>
            <td>{failure.sourcePod}</td>
            <td>
              {failure.targetPod}
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
              {failure.skipped ? "skipped" : `${failure.httpOK ? "ok" : "failed"} status=${failure.httpStatus ?? 0} ${failure.httpDurationMS}ms`}
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

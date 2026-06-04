import type { NetworkSourceSummary } from "../types";

interface SourceSummaryTableProps {
  sources: NetworkSourceSummary[];
}

export function SourceSummaryTable({ sources }: SourceSummaryTableProps) {
  return (
    <table>
      <thead>
        <tr>
          <th>Source Pod</th>
          <th>Targets</th>
          <th>Success</th>
          <th>Failed</th>
          <th>Skipped</th>
          <th>Ping OK</th>
          <th>HTTP OK</th>
          <th>Failure Details</th>
        </tr>
      </thead>
      <tbody>
        {sources.map((source) => (
          <tr key={source.sourcePod}>
            <td>{source.sourcePod}</td>
            <td>{source.targetCount}</td>
            <td className="ok">{source.successCount}</td>
            <td className={source.failedCount ? "failed" : ""}>{source.failedCount}</td>
            <td>{source.skippedCount}</td>
            <td>{source.pingOKCount}</td>
            <td>{source.httpOKCount}</td>
            <td>
              {source.failures.length ? (
                <details>
                  <summary>{source.failedCount} failed targets</summary>
                  <table>
                    <thead>
                      <tr>
                        <th>Target</th>
                        <th>Ping</th>
                        <th>HTTP</th>
                        <th>Reason</th>
                      </tr>
                    </thead>
                    <tbody>
                      {source.failures.map((failure) => (
                        <tr key={`${source.sourcePod}-${failure.targetPod}-${failure.targetIP}`}>
                          <td>
                            {failure.targetPod}
                            <br />
                            <span className="meta">
                              {failure.targetIP} {failure.targetNode}
                            </span>
                          </td>
                          <td className={failure.pingOK ? "ok" : "failed"}>{failure.skipped ? "skipped" : `${failure.pingOK ? "ok" : "failed"} ${failure.pingDurationMS}ms`}</td>
                          <td className={failure.httpOK ? "ok" : "failed"}>
                            {failure.skipped ? "skipped" : `${failure.httpOK ? "ok" : "failed"} status=${failure.httpStatus ?? 0} ${failure.httpDurationMS}ms`}
                          </td>
                          <td>
                            <pre>{`${failure.pingError ?? ""}${failure.httpError ?? ""}${failure.skipReason ?? ""}`}</pre>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </details>
              ) : (
                <span className="ok">all targets ok</span>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

import type { NetworkCheckResult, NetworkCheckSummary, NetworkSourceSummary, NetworkView } from "./types";

export function buildNetworkView(summary: NetworkCheckSummary, layer?: string): NetworkView {
  const results = (summary.results ?? []).filter((result) => !layer || result.layer === layer);
  const view: NetworkView = {
    stats: {
      agentCount: summary.agentCount,
      totalChecks: results.length,
      successCount: 0,
      failedCount: 0,
      skippedCount: 0,
      pingFailed: 0,
      httpFailed: 0,
    },
    sources: [],
    failures: [],
  };

  const bySource = new Map<string, NetworkSourceSummary>();
  for (const result of results) {
    const sourceName = result.sourcePod || "unknown";
    let source = bySource.get(sourceName);
    if (!source) {
      source = {
        sourcePod: sourceName,
        targetCount: 0,
        successCount: 0,
        failedCount: 0,
        skippedCount: 0,
        pingOKCount: 0,
        httpOKCount: 0,
        failures: [],
      };
      bySource.set(sourceName, source);
      view.sources.push(source);
    }

    source.targetCount += 1;
    if (result.skipped) {
      source.skippedCount += 1;
      view.stats.skippedCount += 1;
    }
    if (result.pingOK) {
      source.pingOKCount += 1;
    }
    if (result.httpOK) {
      source.httpOKCount += 1;
    }

    if (networkResultOK(result)) {
      source.successCount += 1;
      view.stats.successCount += 1;
      continue;
    }

    source.failedCount += 1;
    source.failures.push(result);
    view.failures.push(result);
    view.stats.failedCount += 1;
    if (!result.skipped && !result.pingOK) {
      view.stats.pingFailed += 1;
    }
    if (!result.skipped && httpRequired(result) && !result.httpOK) {
      view.stats.httpFailed += 1;
    }
  }

  return view;
}

function networkResultOK(result: NetworkCheckResult): boolean {
  return !result.skipped && result.pingOK && (!httpRequired(result) || result.httpOK);
}

function httpRequired(result: NetworkCheckResult): boolean {
  return !result.layer || result.layer === "pod-to-pod" || result.layer === "node-to-pod";
}

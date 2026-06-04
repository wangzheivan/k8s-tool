import type { AgentsResponse, NetworkCheckSummary } from "./types";

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`${response.status} ${response.statusText}: ${text}`);
  }
  return response.json() as Promise<T>;
}

export function fetchAgents(): Promise<AgentsResponse> {
  return requestJSON<AgentsResponse>("/api/agents");
}

export function refreshAgents(): Promise<AgentsResponse> {
  return requestJSON<AgentsResponse>("/api/refresh", { method: "POST" });
}

export function fetchNetworkCheck(): Promise<NetworkCheckSummary> {
  return requestJSON<NetworkCheckSummary>("/api/network-check");
}

export function runNetworkCheck(): Promise<NetworkCheckSummary> {
  return requestJSON<NetworkCheckSummary>("/api/network-check", { method: "POST" });
}

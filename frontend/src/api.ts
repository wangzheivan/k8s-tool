import type { AgentsResponse, CertStatusSummary, EtcdStatusSummary, NetworkCheckSummary } from "./types";

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

export function fetchLayeredNetworkCheck(): Promise<NetworkCheckSummary> {
  return requestJSON<NetworkCheckSummary>("/api/layered-network-check");
}

export function runLayeredNetworkCheck(): Promise<NetworkCheckSummary> {
  return requestJSON<NetworkCheckSummary>("/api/layered-network-check", { method: "POST" });
}

export function fetchEtcdStatus(): Promise<EtcdStatusSummary> {
  return requestJSON<EtcdStatusSummary>("/api/etcd/status");
}

export function runEtcdStatus(): Promise<EtcdStatusSummary> {
  return requestJSON<EtcdStatusSummary>("/api/etcd/status", { method: "POST" });
}

export function fetchCertStatus(): Promise<CertStatusSummary> {
  return requestJSON<CertStatusSummary>("/api/certs/status");
}

export function runCertStatus(): Promise<CertStatusSummary> {
  return requestJSON<CertStatusSummary>("/api/certs/status", { method: "POST" });
}

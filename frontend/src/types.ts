export interface AgentInfo {
  podName: string;
  namespace: string;
  podIP: string;
  nodeName: string;
  nodeIP: string;
  hostname: string;
  phase: string;
  agentURL: string;
  memoryTotalKB: number;
  memoryFreeKB: number;
  memoryUsedKB: number;
  memoryTotalGB: string;
  memoryFreeGB: string;
  memoryUsedGB: string;
  collectedAt: string;
  status: string;
  error?: string;
  lastRefreshAt: string;
}

export interface AgentsResponse {
  agents: AgentInfo[];
  lastError?: string;
}

export interface NetworkCheckResult {
  layer?: string;
  sourcePod: string;
  sourceNode?: string;
  sourceIP?: string;
  targetName?: string;
  targetPod: string;
  targetIP: string;
  targetNode: string;
  pingOK: boolean;
  pingDurationMS: number;
  pingError?: string;
  httpOK: boolean;
  httpStatus?: number;
  httpDurationMS: number;
  httpError?: string;
  checkedAt: string;
  skipped?: boolean;
  skipReason?: string;
}

export interface NetworkCheckSummary {
  running: boolean;
  startedAt?: string;
  completedAt?: string;
  error?: string;
  agentCount: number;
  results: NetworkCheckResult[];
  sourceErrors?: string[];
}

export interface NetworkStats {
  agentCount: number;
  totalChecks: number;
  successCount: number;
  failedCount: number;
  skippedCount: number;
  pingFailed: number;
  httpFailed: number;
}

export interface NetworkSourceSummary {
  sourcePod: string;
  targetCount: number;
  successCount: number;
  failedCount: number;
  skippedCount: number;
  pingOKCount: number;
  httpOKCount: number;
  failures: NetworkCheckResult[];
}

export interface NetworkView {
  stats: NetworkStats;
  sources: NetworkSourceSummary[];
  failures: NetworkCheckResult[];
}

export interface EtcdCommandResult {
  name: string;
  command: string;
  exitCode: number;
  stdout: string;
  stderr: string;
  durationMS: number;
  error?: string;
}

export interface EtcdStatusResult {
  nodeName: string;
  nodeIP: string;
  agentPod?: string;
  agentPodIP?: string;
  agentURL?: string;
  status: string;
  message?: string;
  etcdContainerID?: string;
  checkedAt: string;
  durationMS: number;
  alarmCount: number;
  commands: EtcdCommandResult[];
}

export interface EtcdStatusSummary {
  running: boolean;
  startedAt?: string;
  completedAt?: string;
  error?: string;
  etcdNodeCount: number;
  checkedNodeCount: number;
  healthyNodeCount: number;
  unhealthyNodeCount: number;
  alarmCount: number;
  results: EtcdStatusResult[];
  sourceErrors?: string[];
}

export interface CertificateInfo {
  nodeName: string;
  nodeIP: string;
  category: string;
  path: string;
  subject: string;
  issuer: string;
  notBefore: string;
  notAfter: string;
  daysLeft: number;
  expired: boolean;
  expiringSoon: boolean;
  parseError?: string;
  status: string;
}

export interface CertNodeResult {
  nodeName: string;
  nodeIP: string;
  role: string;
  agentPod?: string;
  agentPodIP?: string;
  agentURL?: string;
  status: string;
  message?: string;
  checkedAt: string;
  durationMS: number;
  serverCertCount: number;
  agentCertCount: number;
  expiredCount: number;
  expiringSoonCount: number;
  parseErrorCount: number;
  certificates: CertificateInfo[];
}

export interface CertStatusSummary {
  running: boolean;
  startedAt?: string;
  completedAt?: string;
  error?: string;
  nodeCount: number;
  serverNodeCount: number;
  workerNodeCount: number;
  checkedNodeCount: number;
  totalCertCount: number;
  expiredCount: number;
  expiringSoonCount: number;
  parseErrorCount: number;
  results: CertNodeResult[];
  sourceErrors?: string[];
}

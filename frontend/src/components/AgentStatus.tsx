import type { AgentInfo } from "../types";

interface AgentStatusProps {
  agents: AgentInfo[];
}

export function AgentStatus({ agents }: AgentStatusProps) {
  return (
    <section>
      <div className="section-head">
        <div>
          <h2>Agent Status</h2>
          <div className="meta">All discovered agent pods are shown, including failed or non-running pods.</div>
        </div>
      </div>
      {agents.length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>Status</th>
              <th>Pod Name</th>
              <th>Node</th>
              <th>Pod IP</th>
              <th>Node IP</th>
              <th>Memory GiB</th>
              <th>Last Refresh</th>
            </tr>
          </thead>
          <tbody>
            {agents.map((agent) => (
              <tr key={`${agent.namespace}/${agent.podName}`}>
                <td className={`status ${agent.status}`}>{agent.status}</td>
                <td>
                  {agent.podName}
                  <details>
                    <summary>Details</summary>
                    <pre>{`namespace: ${agent.namespace}
phase: ${agent.phase}
agentURL: ${agent.agentURL}
hostname: ${agent.hostname}
collectedAt: ${agent.collectedAt}
memoryTotalKB: ${agent.memoryTotalKB}
memoryUsedKB: ${agent.memoryUsedKB}
memoryFreeKB: ${agent.memoryFreeKB}
error: ${agent.error ?? ""}`}</pre>
                  </details>
                </td>
                <td>{agent.nodeName}</td>
                <td>{agent.podIP}</td>
                <td>{agent.nodeIP}</td>
                <td>
                  {agent.memoryUsedGB} / {agent.memoryTotalGB}
                  <br />
                  free {agent.memoryFreeGB}
                </td>
                <td>{agent.lastRefreshAt}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <div className="empty">No agent data available.</div>
      )}
    </section>
  );
}

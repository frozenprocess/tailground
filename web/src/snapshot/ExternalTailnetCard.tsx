import type { ExternalNode, ExternalTailnet, ExternalTailnetView } from "../engine/types";
import { splitList } from "./splitList";

// One external tailnet (declarative node sharing, C-H): the local alias plus a
// mini-table of shared nodes (name, home-tailnet tags/groups). The policy-side
// config (externalID, allowIncomingConnections) is shown read-only from the
// engine result for context.
export function ExternalTailnetCard({
  tailnet,
  config,
  onChange,
  onDelete,
}: {
  tailnet: ExternalTailnet;
  config: ExternalTailnetView | undefined;
  onChange: (t: ExternalTailnet) => void;
  onDelete: () => void;
}) {
  const nodes = tailnet.nodes ?? [];
  const setNode = (j: number, patch: Partial<ExternalNode>) => {
    const next = nodes.slice();
    next[j] = { ...next[j], ...patch };
    onChange({ ...tailnet, nodes: next });
  };

  return (
    <div
      style={{
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-sm)",
        padding: 8,
        marginBottom: 8,
      }}
    >
      <div style={{ display: "flex", gap: 6, alignItems: "center", marginBottom: 6 }}>
        <input
          type="text"
          style={{ flex: 1 }}
          placeholder="partner"
          value={tailnet.alias}
          onChange={(e) => onChange({ ...tailnet, alias: e.target.value.trim() })}
        />
        {config?.configured ? (
          <span className="eyebrow" title="from the policy's externalTailnets section">
            {config.allowIncomingConnections ? "incoming ✓" : "incoming ✕"}
            {config.externalId ? ` · ${config.externalId}` : ""}
          </span>
        ) : (
          <span
            className="eyebrow"
            style={{ color: "var(--color-check)" }}
            title="add an externalTailnets entry for this alias in the policy"
          >
            no policy entry
          </span>
        )}
        <button className="row-del" onClick={onDelete}>
          ✕
        </button>
      </div>

      <table>
        <thead>
          <tr>
            <th>node</th>
            <th>tags</th>
            <th>groups</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {nodes.map((n, j) => (
            <tr key={j}>
              <td>
                <input
                  type="text"
                  placeholder="eng-laptop"
                  value={n.name}
                  onChange={(e) => setNode(j, { name: e.target.value })}
                />
              </td>
              <td className="tags-cell">
                <input
                  type="text"
                  placeholder="relay"
                  value={(n.tags ?? []).join(", ")}
                  onChange={(e) => setNode(j, { tags: splitList(e.target.value) })}
                />
              </td>
              <td className="tags-cell">
                <input
                  type="text"
                  placeholder="eng"
                  value={(n.groups ?? []).join(", ")}
                  onChange={(e) => setNode(j, { groups: splitList(e.target.value) })}
                />
              </td>
              <td>
                <button
                  className="row-del"
                  onClick={() =>
                    onChange({ ...tailnet, nodes: nodes.filter((_, k) => k !== j) })
                  }
                >
                  ✕
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <button
        className="add-row"
        onClick={() =>
          onChange({
            ...tailnet,
            nodes: [...nodes, { name: `node-${nodes.length + 1}` }],
          })
        }
      >
        + node
      </button>
    </div>
  );
}

import { EDGE_STYLES, LEGEND_DASH, NODE_LEGEND, type EdgeKind } from "./style";

// Rendered straight from the style table so it can never drift from the edges.
export function Legend({ present, directed }: { present: Set<EdgeKind>; directed: boolean }) {
  const kinds = (Object.keys(EDGE_STYLES) as EdgeKind[]).filter((k) => present.has(k));
  return (
    <div className="graph-legend">
      {kinds.map((k) => {
        const s = EDGE_STYLES[k];
        return (
          <span key={k} className="legend-item">
            <svg width="26" height="10" aria-hidden>
              <line
                x1="1"
                y1="5"
                x2="25"
                y2="5"
                stroke={s.stroke}
                strokeWidth={s.width + 0.5}
                strokeDasharray={s.dashed ? LEGEND_DASH : undefined}
              />
            </svg>
            {s.label}
          </span>
        );
      })}
      {directed && (
        <span className="legend-item" title="Replies ride the same connection back">
          <svg width="26" height="10" aria-hidden>
            <line x1="1" y1="5" x2="20" y2="5" stroke="var(--text-muted)" strokeWidth="1.5" />
            <path d="M18,1 L25,5 L18,9 z" fill="currentColor" style={{ fill: "var(--text-muted)" }} />
          </svg>
          arrow = who opens the connection (⇄ both ways)
        </span>
      )}
      {directed && (
        <span
          className="legend-item"
          title="Boxes mark where the drawn arrows end (ingress, left) and start (egress, right)"
        >
          <span className="legend-port gport on in" /> ingress
          <span className="legend-port gport on out" /> egress
          <span className="legend-port gport on both" /> both ways
        </span>
      )}
      {kinds.length > 0 && <span className="legend-sep" />}
      {NODE_LEGEND.map((n) => (
        <span key={n.label} className="legend-item">
          <span
            className="legend-swatch"
            style={{
              background: n.swatch,
              border: n.swatch === "transparent" ? "1px dashed var(--border-strong)" : "1px solid var(--border-strong)",
            }}
          />
          {n.label}
        </span>
      ))}
    </div>
  );
}

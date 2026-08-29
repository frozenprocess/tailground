import { useStore } from "../state/store";
import { ProbeEditor } from "../matrix/ProbeEditor";

const GROUPINGS = [
  { id: "user", label: "By owner" },
  { id: "tag", label: "By tag" },
  { id: "flat", label: "Flat" },
] as const;

/** Which probe the graph shows (or "any"), the probe editor, and the grouping. */
export function ProbeBar({ probeNames }: { probeNames: string[] }) {
  const activeProbe = useStore((s) => s.activeProbe);
  const setActiveProbe = useStore((s) => s.setActiveProbe);
  const anyProbe = useStore((s) => s.graphAnyProbe);
  const setAnyProbe = useStore((s) => s.setGraphAnyProbe);
  const grouping = useStore((s) => s.grouping);
  const setGrouping = useStore((s) => s.setGrouping);

  return (
    <div className="matrix-toolbar">
      <div className="seg">
        {probeNames.map((p) => (
          <button
            key={p}
            className={!anyProbe && p === activeProbe ? "active" : ""}
            onClick={() => {
              setAnyProbe(false);
              setActiveProbe(p);
            }}
          >
            {p}
          </button>
        ))}
        {probeNames.length > 1 && (
          <button
            className={anyProbe ? "active" : ""}
            title="An edge if the pair is reachable on ANY configured port"
            onClick={() => setAnyProbe(!anyProbe)}
          >
            any
          </button>
        )}
      </div>
      <ProbeEditor />
      <span className="spacer" style={{ flex: 1 }} />
      <div className="seg">
        {GROUPINGS.map((g) => (
          <button
            key={g.id}
            className={g.id === grouping ? "active" : ""}
            onClick={() => setGrouping(g.id)}
          >
            {g.label}
          </button>
        ))}
      </div>
    </div>
  );
}

/** The focus hint plus the declutter toggles, group highlight and unpin. */
export function FocusBar({
  focusedLabel,
  reach,
  nodePinned,
  hasPin,
  groupNames,
  inGroupCount,
  onUnpin,
}: {
  focusedLabel: string | null;
  reach: { out: number; in: number } | null;
  nodePinned: boolean;
  hasPin: boolean;
  groupNames: string[];
  inGroupCount: number;
  onUnpin: () => void;
}) {
  const emphasize = useStore((s) => s.graphEmphasize);
  const setEmphasize = useStore((s) => s.setGraphEmphasize);
  const hideIsolated = useStore((s) => s.graphHideIsolated);
  const setHideIsolated = useStore((s) => s.setGraphHideIsolated);
  const highlightGroup = useStore((s) => s.graphHighlightGroup);
  const setHighlightGroup = useStore((s) => s.setGraphHighlightGroup);

  return (
    <div className="matrix-toolbar" style={{ borderTop: "none" }}>
      <span className="graph-hint">
        {focusedLabel && reach ? (
          <>
            <strong>{focusedLabel}</strong> can reach {reach.out} · reachable from {reach.in}
            {nodePinned ? " · pinned — click the background to release" : ""}
          </>
        ) : hasPin ? (
          "pinned — click the background to release"
        ) : (
          "hover a node to trace its flows · click to pin"
        )}
      </span>
      <span className="spacer" style={{ flex: 1 }} />
      <label className="toggle">
        <input
          type="checkbox"
          checked={emphasize}
          onChange={(e) => setEmphasize(e.target.checked)}
        />
        dim routine flows
      </label>
      <label className="toggle">
        <input
          type="checkbox"
          checked={hideIsolated}
          onChange={(e) => setHideIsolated(e.target.checked)}
        />
        hide isolated
      </label>
      {groupNames.length > 0 && (
        <label className="graph-group-pick">
          <select
            className="graph-group-select"
            value={highlightGroup ?? ""}
            onChange={(e) => setHighlightGroup(e.target.value || null)}
            title="Ring every device whose owner resolves into this group"
          >
            <option value="">highlight group…</option>
            {groupNames.map((g) => (
              <option key={g} value={g}>
                {g}
              </option>
            ))}
          </select>
          {highlightGroup && (
            <span className="graph-group-count">
              {inGroupCount} device{inGroupCount === 1 ? "" : "s"}
            </span>
          )}
        </label>
      )}
      {hasPin && (
        <button className="btn" onClick={onUnpin}>
          unpin ✕
        </button>
      )}
    </div>
  );
}

import { useStore } from "../state/store";

// The right region is one panel with two lenses on the same Result. `view`
// ("matrix" | "graph") is this axis alone — independent of the left panel's
// Policy/Tailnet/Learn lens. `canonicalView` folds both into the URL fragment.
const TABS = [
  { id: "matrix", label: "Matrix" },
  { id: "graph", label: "Graph" },
] as const;

export function ImpactTabs() {
  const view = useStore((s) => s.view);
  const setView = useStore((s) => s.setView);
  const active = view === "graph" ? "graph" : "matrix";

  return (
    <div className="pane-tabs">
      {TABS.map((t) => (
        <button
          key={t.id}
          type="button"
          className={t.id === active ? "active" : ""}
          onClick={() => setView(t.id)}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

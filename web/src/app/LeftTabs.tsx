import { useStore, type LeftTab } from "../state/store";
import { DiagMark } from "../ui/DiagMark";
import { snapshotTargeted } from "../engine/diagnostics";
import type { Diagnostic } from "../engine/types";

const NO_DIAGS: Diagnostic[] = [];

const TABS = [
  { id: "tailnet", label: "Tailnet" },
  { id: "learn", label: "Learn" },
] as const;

/**
 * The Tailnet / Learn strip in the left column's bar. It drives `leftTab`
 * alone — the policy editor (right column, top) and the impact panel (`view`)
 * are separate axes, so switching them never disturbs which left lens is open,
 * and vice versa.
 */
export function LeftTabs() {
  const leftTab = useStore((s) => s.leftTab);
  const setLeftTab = useStore((s) => s.setLeftTab);
  const tailnetDiags = snapshotTargeted(useStore((s) => s.result?.diagnostics ?? NO_DIAGS));

  return (
    <div className="pane-tabs">
      {TABS.map((t) => (
        <button
          key={t.id}
          type="button"
          className={t.id === leftTab ? "active" : ""}
          onClick={() => setLeftTab(t.id as LeftTab)}
        >
          {t.label}
          {t.id === "tailnet" && (
            <DiagMark
              diags={tailnetDiags}
              summary={`${tailnetDiags.length} snapshot issue(s) — marked in the Tailnet tab`}
            />
          )}
        </button>
      ))}
    </div>
  );
}

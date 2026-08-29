import { useStore, type MobilePane } from "../state/store";
import { DiagMark } from "../ui/DiagMark";
import { snapshotTargeted } from "../engine/diagnostics";
import type { Diagnostic } from "../engine/types";

const NO_DIAGS: Diagnostic[] = [];

const ITEMS: { id: MobilePane; label: string; glyph: string }[] = [
  { id: "policy", label: "Policy", glyph: "{ }" },
  { id: "tailnet", label: "Tailnet", glyph: "☰" },
  { id: "matrix", label: "Matrix", glyph: "▦" },
  { id: "graph", label: "Graph", glyph: "❖" },
  { id: "learn", label: "Learn", glyph: "▶" },
];

/**
 * The bottom navigation for the stacked mobile layout. One tap selects the sole
 * visible pane; `setMobilePane` keeps the desktop `leftTab` / `view` axes in
 * step so nothing jumps when the viewport widens again.
 */
export function MobileNav() {
  const pane = useStore((s) => s.mobilePane);
  const setPane = useStore((s) => s.setMobilePane);
  const tailnetDiags = snapshotTargeted(useStore((s) => s.result?.diagnostics ?? NO_DIAGS));

  return (
    <nav className="mobile-nav" aria-label="Views">
      {ITEMS.map((it) => (
        <button
          key={it.id}
          type="button"
          className={it.id === pane ? "active" : ""}
          aria-current={it.id === pane ? "page" : undefined}
          onClick={() => setPane(it.id)}
        >
          <span className="mobile-nav-glyph" aria-hidden="true">
            {it.glyph}
          </span>
          {it.label}
          {it.id === "tailnet" && (
            <DiagMark
              diags={tailnetDiags}
              summary={`${tailnetDiags.length} snapshot issue(s) — marked in the Tailnet tab`}
            />
          )}
        </button>
      ))}
    </nav>
  );
}

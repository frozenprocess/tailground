export const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));

/**
 * Persistent workspace layout (L-D). `cols` are the two column widths as
 * percentages (left tabbed panel, right stack); `rows` split the right stack
 * between the policy editor and the impact pane; `strip` is the analysis
 * strip's share of the impact pane; `maximized` is a pane id taking the whole
 * workspace; `collapsed` tracks the analysis sub-panels. Mirrors the
 * theme-pref pattern: `tg-layout` in localStorage, try/catch everywhere,
 * absence = defaults.
 */
export interface LayoutState {
  /** The two workspace column widths (left tabbed panel, right stack). */
  cols: [number, number];
  /** The right column's vertical split (policy editor, impact pane). */
  rows: [number, number];
  strip: number;
  maximized: string | null;
  collapsed: Record<string, boolean>;
}

const LAYOUT_KEY = "tg-layout";
const MAX_PANES = ["input", "editor", "impact"];
const DEFAULT_LAYOUT: LayoutState = {
  cols: [34, 66],
  rows: [42, 58],
  strip: 0.42,
  maximized: null,
  collapsed: {},
};

const isPair = (v: unknown): v is [number, number] =>
  Array.isArray(v) && v.length === 2 && v.every((n) => typeof n === "number");

export function readLayout(): LayoutState {
  try {
    const raw = localStorage.getItem(LAYOUT_KEY);
    if (raw) {
      const p = JSON.parse(raw) as Partial<LayoutState>;
      return {
        cols: isPair(p.cols) ? p.cols : DEFAULT_LAYOUT.cols,
        rows: isPair(p.rows) ? p.rows : DEFAULT_LAYOUT.rows,
        strip: typeof p.strip === "number" ? clamp(p.strip, 0.15, 0.75) : DEFAULT_LAYOUT.strip,
        maximized:
          typeof p.maximized === "string" && MAX_PANES.includes(p.maximized)
            ? p.maximized
            : null,
        collapsed: p.collapsed && typeof p.collapsed === "object" ? p.collapsed : {},
      };
    }
  } catch {
    /* private mode / corrupt value */
  }
  return DEFAULT_LAYOUT;
}

let layoutWriteTimer: number | undefined;
export function persistLayout(layout: LayoutState): void {
  try {
    window.clearTimeout(layoutWriteTimer);
    layoutWriteTimer = window.setTimeout(() => {
      try {
        localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
      } catch {
        /* ignore */
      }
    }, 200);
  } catch {
    /* ignore */
  }
}

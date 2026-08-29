import { useStore } from "../state/store";

/**
 * Returns a factory for `<Panel>` collapse props backed by the persisted
 * `layout.collapsed` map (L-D). `dflt` is the collapsed state before the user
 * has ever toggled this panel.
 *
 *   const collapse = usePanelCollapse();
 *   <Panel {...collapse("snap-users", true)} title="Users" … />
 */
export function usePanelCollapse() {
  const map = useStore((s) => s.layout.collapsed);
  const set = useStore((s) => s.setPanelCollapsed);
  return (id: string, dflt: boolean) => ({
    collapsible: true as const,
    id,
    collapsed: map[id] ?? dflt,
    onCollapsedChange: (c: boolean) => set(id, c),
  });
}

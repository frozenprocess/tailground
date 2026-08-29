import { useStore } from "../state/store";

/**
 * Wires a top-level workspace pane to the persisted `layout.maximized` state
 * (L-D). Spread the result into `<Panel>`: it renders the ⤢ control and, when
 * this pane is the maximized one, `<Workspace>` gives it the whole area.
 */
export function usePaneMax(id: string) {
  const maximized = useStore((s) => s.layout.maximized === id);
  const toggle = useStore((s) => s.toggleMaximized);
  return { maximized, onToggleMaximize: () => toggle(id) };
}

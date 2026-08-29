import type { ReactNode } from "react";
import { PanelGroup, Panel as SplitPanel, PanelResizeHandle } from "react-resizable-panels";
import { useStore } from "../state/store";

/**
 * The resizable workspace (L-D + L-E, reworked 2026-08-29). Two columns: a
 * tabbed left panel (Tailnet / Learn) and a right column that stacks the policy
 * editor over the impact pane (Matrix / Graph). The column divide persists to
 * `layout.cols`, the right-stack divide to `layout.rows`. `layout.maximized`
 * ("input" | "editor" | "impact") gives one pane the whole area, splitters gone.
 */
export function Workspace({
  left,
  editor,
  impact,
}: {
  left: ReactNode;
  editor: ReactNode;
  impact: ReactNode;
}) {
  const cols = useStore((s) => s.layout.cols);
  const rows = useStore((s) => s.layout.rows);
  const maximized = useStore((s) => s.layout.maximized);
  const setCols = useStore((s) => s.setLayoutCols);
  const setRows = useStore((s) => s.setLayoutRows);

  if (maximized) {
    const solo =
      maximized === "impact" ? impact : maximized === "editor" ? editor : left;
    return <div className="workspace-solo">{solo}</div>;
  }

  const right = (
    <PanelGroup direction="vertical" className="right-stack" onLayout={setRows} key="right-stack">
      <SplitPanel id="row-editor" order={1} defaultSize={rows[0]} minSize={15} className="pane-slot">
        {editor}
      </SplitPanel>
      <PanelResizeHandle className="rz rz-v" />
      <SplitPanel id="row-impact" order={2} defaultSize={rows[1]} minSize={20} className="pane-slot">
        {impact}
      </SplitPanel>
    </PanelGroup>
  );

  return (
    <PanelGroup direction="horizontal" className="workspace" onLayout={setCols} key="workspace">
      <SplitPanel id="col-input" order={1} defaultSize={cols[0]} minSize={20} className="pane-slot">
        {left}
      </SplitPanel>
      <PanelResizeHandle className="rz" />
      <SplitPanel id="col-right" order={2} defaultSize={cols[1]} minSize={30} className="pane-slot">
        {right}
      </SplitPanel>
    </PanelGroup>
  );
}

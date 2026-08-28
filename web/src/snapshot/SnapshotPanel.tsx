import { useState } from "react";
import { useStore } from "../state/store";
import type { Snapshot } from "../engine/types";
import { SNAPSHOT_PRESETS } from "../state/presets";
import { Panel } from "../ui/Panel";
import { usePaneMax } from "../app/usePaneMax";
import { usePanelCollapse } from "../app/usePanelCollapse";
import { LeftTabs } from "../app/LeftTabs";
import { SnapshotJson } from "./SnapshotJson";
import { DevicesSection, ExternalSection, ServicesSection, UsersSection } from "./Sections";
import { GroupsHostsEditor } from "./GroupsHostsEditor";

export function SnapshotPanel() {
  const snapshot = useStore((s) => s.snapshot);
  const setSnapshot = useStore((s) => s.setSnapshot);
  const loadPreset = useStore((s) => s.loadPreset);
  const collapse = usePanelCollapse();
  const [mode, setMode] = useState<"form" | "json">("form");

  const update = (patch: Partial<Snapshot>) => setSnapshot({ ...snapshot, ...patch });

  return (
    <Panel
      id="input"
      title="Tailnet snapshot"
      tabs={<LeftTabs />}
      {...usePaneMax("input")}
      toolbar={
        <div className="pane-head">
          <div className="seg">
            <button
              type="button"
              className={mode === "form" ? "active" : ""}
              onClick={() => setMode("form")}
            >
              Form
            </button>
            <button
              type="button"
              className={mode === "json" ? "active" : ""}
              onClick={() => setMode("json")}
            >
              JSON
            </button>
          </div>
        </div>
      }
    >
      {mode === "json" ? (
        <SnapshotJson />
      ) : (
        <div className="pane-body snap">
          <div className="snap-presets">
            <span className="eyebrow">Seed</span>
            {SNAPSHOT_PRESETS.map((p) => (
              <button key={p.id} className="pill" title={p.blurb} onClick={() => loadPreset(p.snapshot)}>
                {p.label}
              </button>
            ))}
          </div>
          <DevicesSection collapse={collapse} />
          <UsersSection collapse={collapse} />
          <ServicesSection collapse={collapse} />
          <ExternalSection collapse={collapse} />
          <GroupsHostsEditor snapshot={snapshot} onChange={update} collapse={collapse} />
        </div>
      )}
    </Panel>
  );
}

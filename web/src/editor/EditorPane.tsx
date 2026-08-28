import { useStore } from "../state/store";
import { POLICY_TEMPLATES } from "../state/presets";
import { Panel } from "../ui/Panel";
import { usePaneMax } from "../app/usePaneMax";
import { PolicyEditor } from "./PolicyEditor";

export function EditorPane() {
  const loadTemplate = useStore((s) => s.loadTemplate);

  return (
    <Panel
      id="editor"
      title="Policy"
      {...usePaneMax("editor")}
      badge={
        <select
          className="pane-bar-select"
          defaultValue=""
          onChange={(e) => {
            const t = POLICY_TEMPLATES.find((x) => x.id === e.target.value);
            if (t) loadTemplate(t.policy);
            e.currentTarget.value = "";
          }}
          title="Load a template"
        >
          <option value="" disabled>
            Load template…
          </option>
          {POLICY_TEMPLATES.map((t) => (
            <option key={t.id} value={t.id} title={t.blurb}>
              {t.label}
            </option>
          ))}
        </select>
      }
    >
      <PolicyEditor />
    </Panel>
  );
}

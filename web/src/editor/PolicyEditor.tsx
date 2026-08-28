import { useEffect, useRef } from "react";
import CodeMirror, { type ReactCodeMirrorRef } from "@uiw/react-codemirror";
import { json } from "@codemirror/lang-json";
import { lintGutter } from "@codemirror/lint";
import { EditorView } from "@codemirror/view";
import { useStore } from "../state/store";
import { policyLint, setEngineDiagnostics } from "./lint";
import { policyCompletion } from "./complete";

const extensions = [
  json(),
  lintGutter(),
  policyLint(),
  policyCompletion(),
  EditorView.lineWrapping,
];

export function PolicyEditor() {
  const policy = useStore((s) => s.policy);
  const setPolicy = useStore((s) => s.setPolicy);
  const result = useStore((s) => s.result);
  const theme = useStore((s) => s.themeResolved);
  const ref = useRef<ReactCodeMirrorRef>(null);

  // Push engine diagnostics into the editor whenever a result lands.
  useEffect(() => {
    const view = ref.current?.view;
    if (!view) return;
    view.dispatch({ effects: setEngineDiagnostics.of(result?.diagnostics ?? []) });
  }, [result]);

  return (
    <div className="editor-wrap">
      <CodeMirror
        ref={ref}
        className="cm-theme"
        theme={theme}
        value={policy}
        extensions={extensions}
        onChange={setPolicy}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: false,
          bracketMatching: true,
          closeBrackets: true,
        }}
      />
    </div>
  );
}

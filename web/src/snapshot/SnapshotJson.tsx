import { useEffect, useMemo, useRef, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { json } from "@codemirror/lang-json";
import { lintGutter } from "@codemirror/lint";
import { EditorView } from "@codemirror/view";
import { useStore } from "../state/store";
import type { Snapshot } from "../engine/types";

const extensions = [json(), lintGutter(), EditorView.lineWrapping];

/**
 * The raw view of the tailnet snapshot. The structured object in the store is
 * still the source of truth — this just renders `JSON.stringify(snapshot)` and
 * writes edits back when they parse to a well-formed snapshot.
 *
 * Input is HuJSON-tolerant (line / block comments and trailing commas are
 * stripped before parsing) so a commented snapshot pastes cleanly, but the
 * canonical form is plain JSON: switching to the form view, loading a preset, or
 * any form edit reformats and drops comments.
 */
export function SnapshotJson() {
  const snapshot = useStore((s) => s.snapshot);
  const setSnapshot = useStore((s) => s.setSnapshot);
  const theme = useStore((s) => s.themeResolved);

  const canonical = useMemo(() => JSON.stringify(snapshot, null, 2), [snapshot]);
  const [text, setText] = useState(canonical);
  const [error, setError] = useState<string | null>(null);
  // The last canonical string we either seeded from or pushed to the store, so a
  // store change that is merely the echo of our own edit doesn't wipe the buffer
  // (and its comments / spacing).
  const mine = useRef(canonical);

  useEffect(() => {
    if (canonical !== mine.current) {
      setText(canonical);
      mine.current = canonical;
      setError(null);
    }
  }, [canonical]);

  const onChange = (next: string) => {
    setText(next);
    let obj: unknown;
    try {
      obj = parseSnapshot(next);
    } catch (e) {
      setError((e as Error).message.replace(/^JSON5?:?\s*/, ""));
      return;
    }
    const bad = validate(obj);
    if (bad) {
      setError(bad);
      return;
    }
    setError(null);
    mine.current = JSON.stringify(obj, null, 2);
    setSnapshot(obj as Snapshot);
  };

  return (
    <div className="editor-wrap">
      <CodeMirror
        className="cm-theme"
        theme={theme}
        value={text}
        extensions={extensions}
        onChange={onChange}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: false,
          bracketMatching: true,
          closeBrackets: true,
        }}
      />
      {error && (
        <div className="snap-json-error" role="alert">
          {error} — the form view still shows the last valid snapshot.
        </div>
      )}
    </div>
  );
}

/** Plain JSON first (the common case); fall back to a HuJSON-tolerant parse. */
function parseSnapshot(src: string): unknown {
  try {
    return JSON.parse(src);
  } catch {
    const stripped = src
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/(^|[^:"'\\])\/\/[^\n\r]*/g, "$1")
      .replace(/,(\s*[}\]])/g, "$1");
    return JSON.parse(stripped);
  }
}

function validate(obj: unknown): string | null {
  if (!obj || typeof obj !== "object" || Array.isArray(obj)) {
    return "A snapshot must be a JSON object.";
  }
  const s = obj as Record<string, unknown>;
  if (!Array.isArray(s.users)) return "Missing a `users` array.";
  if (!Array.isArray(s.devices)) return "Missing a `devices` array.";
  for (const d of s.devices) {
    if (!d || typeof d !== "object" || typeof (d as Device).id !== "number") {
      return "Every device needs a numeric `id`.";
    }
  }
  for (const u of s.users) {
    if (!u || typeof u !== "object" || typeof (u as SnapshotUser).id !== "number") {
      return "Every user needs a numeric `id`.";
    }
  }
  return null;
}

type Device = { id: number };
type SnapshotUser = { id: number };

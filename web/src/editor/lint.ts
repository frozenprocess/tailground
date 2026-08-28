import { StateEffect, StateField, type Extension } from "@codemirror/state";
import { linter, type Diagnostic as CMDiagnostic } from "@codemirror/lint";
import type { EditorView } from "@codemirror/view";
import type { Diagnostic as EngineDiagnostic } from "../engine/types";
import { DIAG_CODE_LABEL } from "../engine/diagnostics";

// ---- engine diagnostics: pushed imperatively, surfaced through the linter ----
//
// The engine is the single source of policy diagnostics (it re-runs on every
// edit), so the editor runs no checks of its own that could repeat one. Only
// positioned diagnostics are drawn here — each at its own token. The rest live
// elsewhere (see engine/diagnostics.ts), so nothing is shown twice.

export const setEngineDiagnostics = StateEffect.define<EngineDiagnostic[]>();

const engineDiagnosticsField = StateField.define<EngineDiagnostic[]>({
  create: () => [],
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setEngineDiagnostics)) return e.value;
    return value;
  },
});

/** "line:col" → a document range: the whole string token when the pointer
 *  lands on a quote, otherwise one character. */
function mapPointer(view: EditorView, pointer: string): [number, number] {
  const doc = view.state.doc;
  const [lineStr, colStr] = pointer.split(":");
  const lineNo = Math.max(1, Math.min(parseInt(lineStr, 10) || 1, doc.lines));
  const line = doc.line(lineNo);
  const col = Math.max(1, parseInt(colStr, 10) || 1);
  const from = Math.min(line.from + col - 1, line.to);
  if (doc.sliceString(from, from + 1) === '"') {
    const close = line.text.indexOf('"', from - line.from + 1);
    if (close >= 0) return [from, line.from + close + 1];
  }
  return [from, Math.min(from + 1, doc.length)];
}

function engineDiagnostics(view: EditorView): CMDiagnostic[] {
  return view.state
    .field(engineDiagnosticsField)
    .filter((d) => !!d.pointer)
    .map((d) => {
      const [from, to] = mapPointer(view, d.pointer!);
      return {
        from,
        to,
        severity: d.severity,
        source: DIAG_CODE_LABEL[d.code] ?? d.code,
        message: d.message,
      };
    });
}

export function policyLint(): Extension {
  return [
    engineDiagnosticsField,
    linter(engineDiagnostics, {
      delay: 250,
      needsRefresh: (update) =>
        update.transactions.some((tr) =>
          tr.effects.some((e) => e.is(setEngineDiagnostics)),
        ),
    }),
  ];
}

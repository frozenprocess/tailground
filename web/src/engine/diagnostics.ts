import type { Diagnostic, DiagnosticCode, DiagnosticSeverity } from "./types";

// Every diagnostic is shown in exactly one place, chosen by what it's about:
//   pointer → at its token in the policy editor (inline lint)
//   target  → on that snapshot item in the Tailnet tab
//   neither → in the matrix / graph pane (an unlocatable compile error or a
//             Tailground bug — rare)

/** How each code reads to the user — what kind of fix it asks for. */
export const DIAG_CODE_LABEL: Record<DiagnosticCode, string> = {
  parse: "policy error",
  policy: "policy issue",
  snapshot: "snapshot gap",
  limit: "simulation limit",
  internal: "Tailground bug",
};

const RANK: Record<DiagnosticSeverity, number> = { error: 0, warning: 1, info: 2 };

/** Diagnostics about one snapshot item ("device:<name>", "services", "external"). */
export function forTarget(ds: Diagnostic[], target: string): Diagnostic[] {
  return ds.filter((d) => d.target === target);
}

/** Every diagnostic that lives in the Tailnet tab. */
export function snapshotTargeted(ds: Diagnostic[]): Diagnostic[] {
  return ds.filter((d) => !!d.target);
}

/** Diagnostics with neither a policy position nor a snapshot target. */
export function unhomed(ds: Diagnostic[]): Diagnostic[] {
  return ds
    .filter((d) => !d.pointer && !d.target)
    .sort((a, b) => RANK[a.severity] - RANK[b.severity]);
}

/** The most severe level among ds (for a mark's colour). */
export function worst(ds: Diagnostic[]): DiagnosticSeverity {
  return ds.reduce<DiagnosticSeverity>(
    (w, d) => (RANK[d.severity] < RANK[w] ? d.severity : w),
    "info",
  );
}

/** A policy-breaking error: nothing can be evaluated until it's fixed. */
export function isFatal(d: Diagnostic): boolean {
  return d.severity === "error" && (d.code === "parse" || d.code === "internal");
}

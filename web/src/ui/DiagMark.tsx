import type { Diagnostic } from "../engine/types";
import { worst } from "../engine/diagnostics";

/**
 * A small warning mark for a snapshot item, its messages in the tooltip. With
 * `summary`, the mark only counts (for a section header whose rows carry the
 * messages), so no text is shown in two places.
 */
export function DiagMark({ diags, summary }: { diags: Diagnostic[]; summary?: string }) {
  if (diags.length === 0) return null;
  const title = summary ?? diags.map((d) => d.message).join("\n");
  return (
    <span className={`diag-mark ${worst(diags)}`} title={title} aria-label={title}>
      {diags.length > 1 ? diags.length : "!"}
    </span>
  );
}

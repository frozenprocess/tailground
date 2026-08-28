import type { Diagnostic } from "../engine/types";
import { DIAG_CODE_LABEL, isFatal, unhomed } from "../engine/diagnostics";
import { useStore } from "../state/store";

const NO_DIAGS: Diagnostic[] = [];

/**
 * Shown in place of the matrix / graph while the policy can't be evaluated.
 * A located error is underlined in the editor, so this only points there; an
 * unlocatable one has no other home, so its message is shown here.
 */
export function FatalNotice({ what }: { what: string }) {
  const diagnostics = useStore((s) => s.result?.diagnostics ?? NO_DIAGS);
  const loose = diagnostics.filter((d) => isFatal(d) && !d.pointer);
  if (loose.length === 0) {
    return (
      <p className="empty">
        The policy has an error — it's underlined in the editor. The {what} updates once it
        compiles.
      </p>
    );
  }
  return (
    <div className="empty engine-notice fatal">
      <p>The policy can't be evaluated:</p>
      {loose.map((d, i) => (
        <p key={i} className="engine-notice-msg">
          {d.message} <span className="code">· {DIAG_CODE_LABEL[d.code] ?? d.code}</span>
        </p>
      ))}
    </div>
  );
}

/**
 * The rare non-fatal diagnostic with neither a policy position nor a snapshot
 * target (e.g. a Tailground bug compiling one device's filter). Its only home.
 */
export function UnhomedNotice() {
  const diagnostics = useStore((s) => s.result?.diagnostics ?? NO_DIAGS);
  const loose = unhomed(diagnostics).filter((d) => !isFatal(d));
  if (loose.length === 0) return null;
  return (
    <div className="engine-notice">
      {loose.map((d, i) => (
        <p key={i} className={`engine-notice-msg ${d.severity}`}>
          {d.message} <span className="code">· {DIAG_CODE_LABEL[d.code] ?? d.code}</span>
        </p>
      ))}
    </div>
  );
}

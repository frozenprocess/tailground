// Pieces the route / service / external-share overlay panels have in common.

import type { EngineResult } from "../engine/types";
import { pairKey } from "../engine/keys";

/** Names of the actors (minus `exclude`) allowed to `column` on the probe. */
export function reachersOf(
  result: EngineResult,
  probe: string,
  column: string,
  exclude: (id: number) => boolean,
): string[] {
  const matrix = result.matrices[probe] ?? {};
  return result.actors
    .filter((a) => !exclude(a.id) && matrix[pairKey(a.id, column)] === "allow")
    .map((a) => a.name);
}

/** Device names as a row of code chips. */
export function NameChips({ names }: { names: string[] }) {
  return (
    <>
      {names.map((n) => (
        <code key={n} style={{ marginRight: 4 }}>
          {n}
        </code>
      ))}
    </>
  );
}

/** " — reachable on <probe> from …", or that no source reaches the target. */
export function ReachedFrom({ reachers, probe }: { reachers: string[]; probe: string }) {
  return reachers.length > 0 ? (
    <>
      {" "}
      — reachable on <code>{probe}</code> from <NameChips names={reachers} />
    </>
  ) : (
    <>
      {" "}
      — no source reaches it on <code>{probe}</code>
    </>
  );
}

export function OverlayWarnings({ warnings }: { warnings?: string[] }) {
  if (!warnings || warnings.length === 0) return null;
  return (
    <div>
      <div className="eyebrow" style={{ color: "var(--color-check)" }}>
        Warnings
      </div>
      <ul>
        {warnings.map((w, i) => (
          <li key={i}>{w}</li>
        ))}
      </ul>
    </div>
  );
}

import type { PostureReport } from "../engine/types";

/** Count used for the strip's collapse badge. */
export function postureCount(posture: PostureReport): number {
  return Object.keys(posture.denials ?? {}).length;
}

/**
 * Body of the device-posture overlay: which rules are posture-gated, what each
 * posture requires, and any grammar warnings. Wrapped by `AnalysisStrip` in a
 * collapsible `<Panel>`; rendered wherever the matrix or graph is shown, when
 * the policy declares `postures`.
 */
export function PosturePanel({ posture }: { posture: PostureReport }) {
  return (
      <div className="posture-body">
        {posture.gates.length > 0 && (
          <div>
            <div className="eyebrow">Posture-gated rules</div>
            <ul>
              {posture.gates.map((g) => (
                <li key={g.rule}>
                  <code>{g.rule}</code> requires{" "}
                  {g.postures.map((p, i) => (
                    <span key={p}>
                      {i > 0 && " or "}
                      <code>{p}</code>
                    </span>
                  ))}
                  {g.default && " (from defaultSrcPosture)"}
                </li>
              ))}
            </ul>
          </div>
        )}

        <div>
          <div className="eyebrow">Definitions</div>
          <ul>
            {Object.entries(posture.definitions).map(([name, conds]) => (
              <li key={name}>
                <code>{name}</code>: {conds.join(" and ")}
              </li>
            ))}
          </ul>
        </div>

        {posture.warnings && posture.warnings.length > 0 && (
          <div>
            <div className="eyebrow" style={{ color: "var(--color-check)" }}>Warnings</div>
            <ul>
              {posture.warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        )}

        <p className="posture-disclaimer">
          Posture conditions are evaluated by Tailground's own reading of the
          syntax (<code>node:os</code>, <code>node:tsVersion</code>,{" "}
          <code>node:tsReleaseTrack</code>), not Tailscale's code.
        </p>
      </div>
  );
}

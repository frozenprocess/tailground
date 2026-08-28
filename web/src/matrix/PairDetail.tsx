import type { EngineResult } from "../engine/types";
import { pairKey, splitSSHKey } from "../engine/keys";
import type { PairSelection } from "../state/store";

export function PairDetail({
  result,
  pair,
  onClose,
}: {
  result: EngineResult;
  pair: PairSelection;
  onClose: () => void;
}) {
  const src = result.actors.find((a) => a.id === pair.src);
  const dst = result.actors.find((a) => a.id === pair.dst);
  if (!src || !dst) return null;

  const key = pairKey(pair.src, pair.dst);
  const grants = result.grants?.[key] ?? [];
  const postureDenial = result.posture?.denials?.[key];
  const sshEntries = Object.entries(result.ssh ?? {})
    .filter(([k]) => k.endsWith(`@${key}`))
    .map(([k, v]) => [splitSSHKey(k)[0], v] as const);

  return (
    <div className="pair-detail">
      <h3>
        {src.name} <span style={{ color: "var(--text-subtle)" }}>→</span> {dst.name}
        <button className="close" onClick={onClose} aria-label="close">
          ✕
        </button>
      </h3>

      {postureDenial && (
        <div className="posture-note">
          <strong>Posture layer:</strong> {postureDenial}
          <div className="posture-note-sub">
            Tailground's own interpretation of the posture syntax — not Tailscale's code.
          </div>
        </div>
      )}

      <div className="eyebrow" style={{ marginBottom: 4 }}>Per-probe verdict</div>
      <div className="verdict-list">
        {Object.entries(result.matrices).map(([probe, m]) => {
          const v = m[key] === "allow" ? "allow" : "deny";
          return (
            <span key={probe} className={`verdict-chip ${v}`}>
              {probe}: {v}
            </span>
          );
        })}
      </div>

      {sshEntries.length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>SSH</div>
          <div className="verdict-list">
            {sshEntries.map(([user, verdict]) => (
              <span
                key={user}
                className={`verdict-chip ${verdict === "accept" ? "allow" : "deny"}`}
              >
                {user}: {verdict}
              </span>
            ))}
          </div>
        </>
      )}

      {grants.length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>App capabilities granted</div>
          <div className="verdict-list">
            {grants.map((g) => (
              <span key={g} className="verdict-chip">{g}</span>
            ))}
          </div>
        </>
      )}

      <div className="eyebrow" style={{ margin: "10px 0 4px" }}>Endpoints</div>
      <dl>
        <dt>{src.name}</dt>
        <dd>{src.ips.join(", ") || "—"}</dd>
        <dt>{dst.name}</dt>
        <dd>{dst.ips.join(", ") || "—"}</dd>
      </dl>
    </div>
  );
}

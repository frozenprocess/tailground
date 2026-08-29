import type { EngineResult, FilterRuleView } from "../engine/types";
import { pairEnds, splitSSHKey } from "../engine/keys";
import { useStore } from "../state/store";

const PROTO_NAMES: Record<number, string> = {
  1: "icmp",
  6: "tcp",
  17: "udp",
  58: "icmpv6",
  132: "sctp",
};

/**
 * Single-device view: identity, posture attributes, the routes it advertises,
 * the services it backs, node capabilities, the compiled packet filter this
 * device would enforce (the Tailscale-native artifact), and its inbound SSH
 * rules. Opens when a matrix row/column header is clicked.
 */
export function DeviceInspector({
  result,
  deviceId,
  onClose,
}: {
  result: EngineResult;
  deviceId: number;
  onClose: () => void;
}) {
  const device = useStore((s) => s.snapshot.devices.find((d) => d.id === deviceId));
  const actor = result.actors.find((a) => a.id === deviceId);
  if (!actor) return null;

  const caps = result.nodeCaps?.[String(deviceId)] ?? {};
  const rules: FilterRuleView[] = result.rules?.[String(deviceId)] ?? [];

  // Posture: the device's own attrs (node:os folded in), plus any posture gate
  // it fails as a source.
  const postureAttrs: Record<string, string> = { ...(device?.posture ?? {}) };
  if (device?.os && !postureAttrs["node:os"] && !postureAttrs["os"]) {
    postureAttrs["node:os"] = device.os;
  }
  const postureDenials = Object.entries(result.posture?.denials ?? {})
    .filter(([k]) => pairEnds(k)[0] === String(deviceId))
    .map(([k, reason]) => {
      const dstId = Number(pairEnds(k)[1]);
      const dst = result.actors.find((a) => a.id === dstId);
      return { dst: dst?.name ?? String(dstId), reason };
    });

  const advertised = (result.routes?.paths ?? []).filter(
    (p) => p.routerId === deviceId,
  );
  const backs = (result.services?.paths ?? []).filter((p) =>
    (p.backends ?? []).includes(deviceId),
  );

  const inboundSSH = Object.entries(result.ssh ?? {})
    .filter(([k]) => pairEnds(k)[1] === String(deviceId))
    .map(([k, v]) => {
      const [user, pair] = splitSSHKey(k);
      const srcId = Number(pairEnds(pair)[0]);
      const src = result.actors.find((a) => a.id === srcId);
      return { user, src: src?.name ?? String(srcId), verdict: v };
    });

  return (
    <div className="pair-detail device-inspector">
      <h3>
        {actor.name}
        <span className="kind-chip">{actor.kind}</span>
        <button className="close" onClick={onClose} aria-label="close">
          ✕
        </button>
      </h3>

      <dl>
        {actor.user && (
          <>
            <dt>user</dt>
            <dd>{actor.user}</dd>
          </>
        )}
        {actor.tags && actor.tags.length > 0 && (
          <>
            <dt>tags</dt>
            <dd>{actor.tags.join(" ")}</dd>
          </>
        )}
        <dt>addresses</dt>
        <dd>{actor.ips.join(", ") || "—"}</dd>
      </dl>

      {Object.keys(postureAttrs).length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>
            Device posture
          </div>
          <div className="verdict-list">
            {Object.entries(postureAttrs).map(([k, v]) => (
              <span key={k} className="verdict-chip">
                {k.startsWith("node:") || k.startsWith("custom:") ? k : `node:${k}`} = {v}
              </span>
            ))}
          </div>
          {postureDenials.length > 0 && (
            <div className="verdict-list" style={{ marginTop: 4 }}>
              {postureDenials.map((d, i) => (
                <span key={i} className="verdict-chip deny" title={d.reason}>
                  fails a posture gate reaching {d.dst}
                </span>
              ))}
            </div>
          )}
        </>
      )}

      {advertised.length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>
            Advertised routes
          </div>
          <div className="verdict-list">
            {advertised.map((p, i) => (
              <span
                key={i}
                className={`verdict-chip ${p.approved ? "allow" : "deny"}`}
                title={
                  p.approved
                    ? "advertised and admin-approved — carries traffic"
                    : "advertised but not approved — no traffic"
                }
              >
                {p.prefix}
                {p.exit ? " (exit node)" : ""} —{" "}
                {p.approved ? "approved" : "pending approval"}
              </span>
            ))}
          </div>
        </>
      )}

      {backs.length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>
            Backs Tailscale Services
          </div>
          <div className="verdict-list">
            {backs.map((p, i) => (
              <span key={i} className="verdict-chip">
                {p.name} ({p.vip})
              </span>
            ))}
          </div>
        </>
      )}

      {Object.keys(caps).length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>
            Node capabilities (nodeAttrs)
          </div>
          <div className="verdict-list">
            {Object.entries(caps).map(([name, vals]) => (
              <span key={name} className="verdict-chip" title={vals.join("\n")}>
                {name}
                {vals.length > 0 && vals[0] !== "" ? ` = ${vals.join(", ")}` : ""}
              </span>
            ))}
          </div>
        </>
      )}

      <div className="eyebrow" style={{ margin: "10px 0 4px" }}>
        Compiled packet filter ({rules.length} rule{rules.length === 1 ? "" : "s"})
      </div>
      {rules.length === 0 ? (
        <p className="inspector-empty">
          Nothing may open a connection to this device.
        </p>
      ) : (
        <div className="filter-rules">
          {rules.map((r, i) => (
            <div key={i} className="filter-rule">
              <div className="fr-line">
                <span className="fr-label">from</span>{" "}
                {r.srcIPs.join(", ")}
              </div>
              {r.dstPorts && r.dstPorts.length > 0 && (
                <div className="fr-line">
                  <span className="fr-label">to</span> {r.dstPorts.join(", ")}
                </div>
              )}
              {r.ipProto && r.ipProto.length > 0 && (
                <div className="fr-line">
                  <span className="fr-label">proto</span>{" "}
                  {r.ipProto.map((p) => PROTO_NAMES[p] ?? p).join(", ")}
                </div>
              )}
              {r.capGrant && r.capGrant.length > 0 && (
                <div className="fr-line">
                  <span className="fr-label">grant</span> {r.capGrant.join("; ")}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {inboundSSH.length > 0 && (
        <>
          <div className="eyebrow" style={{ margin: "10px 0 4px" }}>
            Inbound SSH
          </div>
          <div className="verdict-list">
            {inboundSSH.map((e, i) => (
              <span
                key={i}
                className={`verdict-chip ${e.verdict === "accept" ? "allow" : "deny"}`}
              >
                {e.src} as {e.user}: {e.verdict}
              </span>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

import type { EngineResult, ExternalShareReport } from "../engine/types";
import { pairEnds } from "../engine/keys";
import { NameChips, OverlayWarnings } from "./OverlayParts";

/** Count used for the strip's collapse badge. */
export function externalShareCount(share: ExternalShareReport): number {
  return share.tailnets.reduce((n, t) => n + (t.nodes?.length ?? 0), 0);
}

/**
 * Body of the declarative-node-sharing overlay: each external tailnet the policy
 * configures, which local devices its policy may grant access to (from
 * `allowExternalReferencesTo`), its shared nodes, and — on the currently
 * selected probe, when the tailnet allows incoming connections — which local
 * devices each shared node reaches. Wrapped by `AnalysisStrip` in a collapsible
 * `<Panel>`.
 */
export function ExternalSharePanel({
  share,
  result,
  probe,
}: {
  share: ExternalShareReport;
  result: EngineResult;
  probe: string;
}) {
  const matrix = result.matrices[probe] ?? {};
  const nameFor = (id: number) => result.actors.find((a) => a.id === id)?.name ?? `#${id}`;

  return (
      <div className="posture-body">
        {share.tailnets.map((t) => (
          <div key={t.alias} style={{ marginBottom: 8 }}>
            <div>
              <code>{t.alias}</code>
              {t.externalId && <span className="posture-count"> · id {t.externalId}</span>}
              {!t.configured && (
                <span className="posture-count"> · no externalTailnets entry in the policy</span>
              )}
              {t.configured && (
                <span className="posture-count">
                  {" "}
                  · incoming connections {t.allowIncomingConnections ? "allowed" : "blocked"}
                </span>
              )}
            </div>
            {t.configured && (
              <div style={{ marginLeft: 12, fontSize: 12 }}>
                {(t.referenceable ?? []).length > 0 ? (
                  <>
                    its policy may grant access to{" "}
                    {(t.referenceable ?? []).map((id) => (
                      <code key={id} style={{ marginRight: 4 }}>
                        {nameFor(id)}
                      </code>
                    ))}
                    <span className="posture-count">(allowExternalReferencesTo)</span>
                  </>
                ) : (
                  <>
                    its policy can't name any local device — nothing is listed in{" "}
                    <code>allowExternalReferencesTo</code>
                  </>
                )}
              </div>
            )}
            <ul>
              {(t.nodes ?? []).map((n) => {
                const membership = [...(n.tags ?? []), ...(n.groups ?? [])].join(", ");
                const inReached = n.inbound
                  ? Object.keys(matrix)
                      .filter((k) => pairEnds(k)[0] === n.column && matrix[k] === "allow")
                      .map((k) => nameFor(Number(pairEnds(k)[1])))
                  : [];
                return (
                  <li key={n.name}>
                    <code>{n.name}</code>
                    {membership && <span className="posture-count"> ({membership})</span>}{" "}
                    <span className="posture-count">{n.ips[0]}</span>
                    <div style={{ marginLeft: 12, fontSize: 12 }}>
                      {n.inbound ? (
                        inReached.length > 0 ? (
                          <>
                            reaches <NameChips names={inReached} /> on <code>{probe}</code>
                          </>
                        ) : (
                          <>reaches no local device on <code>{probe}</code></>
                        )
                      ) : (
                        <>can't initiate connections here — incoming connections are blocked</>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}

        <OverlayWarnings warnings={share.warnings} />

        <p className="posture-disclaimer">
          A <code>group://</code> / <code>tag://</code> reference is only valid in a
          grant's <code>src</code>, and only when its tailnet sets{" "}
          <code>allowIncomingConnections</code> — anywhere else, Tailscale rejects the
          whole policy, and so does Tailground. Whether your
          devices reach a shared node is decided by the other tailnet's policy,
          which isn't visible here — so that direction gets no verdict, only the
          list of devices <code>allowExternalReferencesTo</code> lets it name.
        </p>
      </div>
  );
}

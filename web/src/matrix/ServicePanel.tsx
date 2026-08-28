import type { EngineResult, ServiceReport } from "../engine/types";
import { OverlayWarnings, ReachedFrom, reachersOf } from "./OverlayParts";

/**
 * Body of the Tailscale Service overlay: each `svc:<name>` in the snapshot, its
 * VIP, the backend devices, and — on the currently selected probe — which
 * sources reach the service. Wrapped by `AnalysisStrip` in a collapsible
 * `<Panel>`; rendered wherever the matrix or graph is shown, when the snapshot
 * defines a service.
 */
export function ServicePanel({
  services,
  result,
  probe,
  onFocusDevice,
}: {
  services: ServiceReport;
  result: EngineResult;
  probe: string;
  onFocusDevice?: (id: number) => void;
}) {
  return (
      <div className="posture-body">
        <ul>
          {services.paths.map((p) => {
            const backendIds = new Set(p.backends ?? []);
            const reachers = p.column
              ? reachersOf(result, probe, p.column, (id) => backendIds.has(id))
              : [];
            const backendList = (p.backends ?? []).map((id) => ({
              id,
              name: result.actors.find((a) => a.id === id)?.name ?? `#${id}`,
            }));
            return (
              <li key={p.name}>
                <code>{p.name}</code> → <code>{p.vip}</code>
                {backendList.length > 0 ? (
                  <>
                    {" "}
                    on{" "}
                    {backendList.map((b, k) => (
                      <span key={b.id}>
                        {k > 0 ? ", " : ""}
                        {onFocusDevice ? (
                          <button
                            className="link-code"
                            onClick={() => onFocusDevice(b.id)}
                          >
                            {b.name}
                          </button>
                        ) : (
                          <code>{b.name}</code>
                        )}
                      </span>
                    ))}
                  </>
                ) : (
                  <span className="posture-count"> no backend device</span>
                )}
                {p.column && <ReachedFrom reachers={reachers} probe={probe} />}
              </li>
            );
          })}
        </ul>

        <OverlayWarnings warnings={services.warnings} />

        <p className="posture-disclaimer">
          The Headscale engine has no notion of Tailscale Services: Tailground
          rewrites each <code>svc:&lt;name&gt;</code> destination to the service
          VIP before compiling, then probes reachability to that VIP against each
          backend device's own filter. Which backend actually serves a
          connection, and how the VIP is provisioned, are not simulated.
        </p>
      </div>
  );
}

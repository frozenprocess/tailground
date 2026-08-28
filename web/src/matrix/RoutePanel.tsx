import type { EngineResult, RouteReport } from "../engine/types";
import { OverlayWarnings, ReachedFrom, reachersOf } from "./OverlayParts";

/**
 * Body of the route / exit-node overlay: which device carries which off-tailnet
 * prefix, whether it's approved, and — for the approved ones — which sources
 * reach through it on the currently selected probe. Wrapped by `AnalysisStrip`
 * in a collapsible `<Panel>`; rendered wherever the matrix or graph is shown,
 * when the snapshot advertises a route.
 */
export function RoutePanel({
  routes,
  result,
  probe,
  onFocusDevice,
}: {
  routes: RouteReport;
  result: EngineResult;
  probe: string;
  onFocusDevice?: (id: number) => void;
}) {
  return (
      <div className="posture-body">
        <ul>
          {routes.paths.map((p) => {
            const reachers = p.column
              ? reachersOf(result, probe, p.column, (id) => id === p.routerId)
              : [];
            return (
              <li key={`${p.prefix}@${p.routerId}`}>
                <code>{p.prefix}</code>
                {p.exit ? " (exit)" : ""} via{" "}
                {onFocusDevice ? (
                  <button className="link-code" onClick={() => onFocusDevice(p.routerId)}>
                    {p.router}
                  </button>
                ) : (
                  <code>{p.router}</code>
                )}
                {!p.approved ? (
                  <span className="posture-count"> advertised, not approved</span>
                ) : (
                  <ReachedFrom reachers={reachers} probe={probe} />
                )}
              </li>
            );
          })}
        </ul>

        <OverlayWarnings warnings={routes.warnings} />

        <p className="posture-disclaimer">
          Reachability to an off-tailnet address is Tailground's overlay: it
          evaluates each source against the router's own compiled packet filter,
          where Headscale places the rule for a CIDR destination or a{" "}
          <code>grants[].via</code> hop. The routing itself is not simulated.
        </p>
      </div>
  );
}

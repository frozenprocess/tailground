import { useRef, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import type { EngineResult } from "../engine/types";
import { useStore, type PairSelection } from "../state/store";
import { usePanelCollapse } from "../app/usePanelCollapse";
import { Panel } from "../ui/Panel";
import { PosturePanel, postureCount } from "../matrix/PosturePanel";
import { RoutePanel } from "../matrix/RoutePanel";
import { ServicePanel } from "../matrix/ServicePanel";
import { ExternalSharePanel, externalShareCount } from "../matrix/ExternalSharePanel";
import { PairDetail } from "../matrix/PairDetail";
import { DeviceInspector } from "../inspector/DeviceInspector";

const LAYER = <span className="layer-tag">Tailground layer</span>;
const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));

/**
 * The analysis strip that sits *below* the matrix grid / graph canvas (L-C).
 * Everything here is derived from the same `Result`: the four Tailground overlay
 * layers (posture / routes / services / node-sharing) as collapsible panels with
 * a count badge, plus the transient PairDetail / DeviceInspector cards.
 *
 * L-D: the strip's height is a drag-persisted fraction of the impact pane
 * (`layout.strip`), and each sub-panel's collapse state persists in
 * `layout.collapsed`.
 *
 * Shared by MatrixView and GraphView. `focusedActor` / `onCloseDevice` are
 * matrix-only (the graph has no device inspector).
 */
export function AnalysisStrip({
  result,
  probe,
  sshMode,
  selectedPair,
  onClosePair,
  focusedActor,
  onCloseDevice,
  onFocusDevice,
}: {
  result: EngineResult;
  probe: string;
  sshMode: boolean;
  selectedPair: PairSelection | null;
  onClosePair: () => void;
  focusedActor?: number | null;
  onCloseDevice?: () => void;
  onFocusDevice?: (id: number) => void;
}) {
  const frac = useStore((s) => s.layout.strip);
  const setFraction = useStore((s) => s.setStripFraction);
  const collapse = usePanelCollapse();
  const stripRef = useRef<HTMLDivElement>(null);
  const drag = useRef<{ startY: number; startFrac: number; parentH: number } | null>(null);

  const overlays = !sshMode;
  const denials = result.posture ? postureCount(result.posture) : 0;

  const hasContent =
    !!result.posture ||
    (overlays && !!result.routes) ||
    (overlays && !!result.services) ||
    (overlays && !!result.externalShare) ||
    !!selectedPair ||
    (focusedActor != null && !!onCloseDevice);

  if (!hasContent) return null;

  const onDragMove = (e: PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    setFraction(clamp(d.startFrac + (d.startY - e.clientY) / d.parentH, 0.15, 0.75));
  };
  const onDragEnd = () => {
    drag.current = null;
    window.removeEventListener("pointermove", onDragMove);
    window.removeEventListener("pointerup", onDragEnd);
  };
  const onDragStart = (e: ReactPointerEvent) => {
    const parent = stripRef.current?.parentElement;
    if (!parent) return;
    e.preventDefault();
    drag.current = { startY: e.clientY, startFrac: frac, parentH: parent.clientHeight };
    window.addEventListener("pointermove", onDragMove);
    window.addEventListener("pointerup", onDragEnd);
  };

  const overlayPanel = (id: string, title: string, count: ReactNode, body: ReactNode, dflt = true) => (
    <Panel
      {...collapse(id, dflt)}
      title={title}
      badge={
        <>
          {LAYER} {count}
        </>
      }
    >
      {body}
    </Panel>
  );

  return (
    <>
      <div
        className="strip-resize"
        role="separator"
        aria-orientation="horizontal"
        aria-label="Resize the analysis panel"
        onPointerDown={onDragStart}
      />
      <div className="analysis-strip" ref={stripRef} style={{ maxHeight: `${frac * 100}%` }}>
        {selectedPair ? (
          <PairDetail result={result} pair={selectedPair} onClose={onClosePair} />
        ) : focusedActor != null && onCloseDevice ? (
          <DeviceInspector result={result} deviceId={focusedActor} onClose={onCloseDevice} />
        ) : null}

        {result.posture &&
          overlayPanel(
            "an-posture",
            "Device posture",
            denials > 0 ? `${denials} downgraded` : "",
            <PosturePanel posture={result.posture} />,
            denials === 0,
          )}

        {overlays &&
          result.routes &&
          overlayPanel(
            "an-routes",
            "Routes & exit nodes",
            result.routes.paths.length,
            <RoutePanel
              routes={result.routes}
              result={result}
              probe={probe}
              onFocusDevice={onFocusDevice}
            />,
          )}

        {overlays &&
          result.services &&
          overlayPanel(
            "an-services",
            "Tailscale Services",
            result.services.paths.length,
            <ServicePanel
              services={result.services}
              result={result}
              probe={probe}
              onFocusDevice={onFocusDevice}
            />,
          )}

        {overlays &&
          result.externalShare &&
          overlayPanel(
            "an-extshare",
            "Declarative node sharing",
            externalShareCount(result.externalShare),
            <ExternalSharePanel share={result.externalShare} result={result} probe={probe} />,
          )}
      </div>
    </>
  );
}

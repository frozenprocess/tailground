import { useEffect, useMemo } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  ViewportPortal,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useStore } from "../state/store";
import { AnalysisStrip } from "../impact/AnalysisStrip";
import {
  buildGraphModel,
  pairOfEdgeId,
  resolveGroupOwners,
  type GraphModel,
} from "./model";
import { reachCounts, visibleEdges } from "./visibility";
import { useGraphLayout } from "./useGraphLayout";
import { useRefitOnSubject } from "./useRefitOnSubject";
import { computeHulls } from "./hulls";
import { nodeTypes, edgeTypes } from "./nodes";
import { portsOf, rfEdges, rfNodes, tailnetFrame } from "./rfElements";
import { Legend } from "./Legend";
import { FocusBar, ProbeBar } from "./GraphToolbars";
import { samePair, useHoverFocus } from "./useHoverFocus";
import type { EdgeKind } from "./style";
import { Panel } from "../ui/Panel";
import { usePaneMax } from "../app/usePaneMax";
import { ImpactTabs } from "../impact/ImpactTabs";
import { FatalNotice, UnhomedNotice } from "../impact/EngineNotice";
import { isFatal } from "../engine/diagnostics";

/** fitView padding when React Flow first mounts (re-fits: useRefitOnSubject). */
const INITIAL_FIT_PADDING = 0.18;

export function GraphView() {
  return (
    <ReactFlowProvider>
      <GraphPane />
    </ReactFlowProvider>
  );
}

function GraphPane() {
  const result = useStore((s) => s.result);
  const probes = useStore((s) => s.probes);
  const activeProbe = useStore((s) => s.activeProbe);
  const grouping = useStore((s) => s.grouping);
  const emphasize = useStore((s) => s.graphEmphasize);
  const hideIsolated = useStore((s) => s.graphHideIsolated);
  const anyProbe = useStore((s) => s.graphAnyProbe);
  const highlightGroup = useStore((s) => s.graphHighlightGroup);
  const setHighlightGroup = useStore((s) => s.setGraphHighlightGroup);
  const snapshot = useStore((s) => s.snapshot);
  const pinnedActor = useStore((s) => s.focusedActor);
  const setFocusedActor = useStore((s) => s.setFocusedActor);
  const pinnedPair = useStore((s) => s.selectedPair);
  const selectPair = useStore((s) => s.selectPair);
  const activeLesson = useStore((s) => s.activeLesson);

  // Hover-to-focus: the pointer previews a node's / edge's neighbourhood; a
  // click pins it (`focusedActor` / `selectedPair` in the store). A pin always
  // wins over a hover so the view stays put while you read it.
  const hover = useHoverFocus();
  const { hoverActor, hoverPair } = hover;
  const focusedActor = pinnedActor ?? hoverActor;
  const selectedPair = pinnedPair ?? hoverPair;
  const hasPin = pinnedActor != null || pinnedPair != null;
  // A bare node hover with nothing pinned — the only state where the edge set is
  // still churning under the cursor, so the only one that needs the edge layer
  // made click-through (below). Once a node or a pair is pinned the graph is
  // stable and flows stay hoverable / clickable.
  const transientHover = hoverActor != null && !hasPin;

  // Any node the pointer can focus — a device, a route/service/shared-node
  // target, or the internet node. Container nodes (group / extframe hulls)
  // carry no actor id and are inert.
  const actorIdOfNode = (n: Node): number | null => {
    const a = (n.data as { actorId?: number | null }).actorId;
    return typeof a === "number" ? a : null;
  };

  const probeNames = result
    ? probes.map((p) => p.name).filter((n) => n in result.matrices)
    : [];
  const probeLabel = (name: string) => {
    const p = probes.find((q) => q.name === name);
    return p ? `${p.proto}${p.port != null ? `:${p.port}` : ""}` : name;
  };

  const fatal = result?.diagnostics?.some(isFatal);

  const model = useMemo<GraphModel | null>(() => {
    if (!result) return null;
    const union = anyProbe
      ? Object.keys(result.matrices).filter((k) => !k.startsWith("ssh:"))
      : undefined;
    return buildGraphModel(result, grouping, activeProbe, union);
  }, [result, grouping, activeProbe, anyProbe]);

  const layout = useGraphLayout(model);
  const { positioned, laying, ready: layoutReady } = layout;

  const frameRect = useMemo(
    () => (model && positioned && layoutReady ? tailnetFrame(model, positioned) : null),
    [model, positioned, layoutReady],
  );

  // P2-5 — identity-cluster hull regions, from the final node positions. A
  // multi-tag device lands inside several overlapping hulls (ELK only parented
  // it to one). Recomputed only on relayout.
  const hulls = useMemo(
    () => (model && positioned && layoutReady ? computeHulls(model, positioned) : []),
    [model, positioned, layoutReady],
  );

  // Which devices connect to the focused one (via any allow edge)?
  const connected = useMemo(() => {
    const set = new Set<number>();
    if (model && focusedActor != null) {
      for (const e of model.skeleton) {
        if (e.src === focusedActor) set.add(e.dst);
        if (e.dst === focusedActor) set.add(e.src);
      }
    }
    return set;
  }, [model, focusedActor]);

  // The focused node's reach, for its badge and the toolbar hint.
  const reach = useMemo(
    () => (model && focusedActor != null ? reachCounts(model, focusedActor) : null),
    [model, focusedActor],
  );
  const focusedLabel =
    focusedActor == null
      ? null
      : (model?.nodes.find((n) => n.actorId === focusedActor)?.label ?? null);

  // Isolated = a device with no allow edge at all (for the declutter toggle).
  const isolated = useMemo(() => {
    const withEdge = new Set<number>();
    for (const e of model?.skeleton ?? []) {
      withEdge.add(e.src);
      withEdge.add(e.dst);
    }
    const set = new Set<number>();
    for (const n of model?.nodes ?? [])
      if (n.kind === "device" && n.actorId != null && !withEdge.has(n.actorId))
        set.add(n.actorId);
    return set;
  }, [model]);
  // The actors "hide isolated" removes from the graph — nodes and edges alike.
  const hidden = useMemo(
    () => (hideIsolated ? isolated : new Set<number>()),
    [hideIsolated, isolated],
  );

  // P2-6 / G-F — `group:` highlight overlay. Groups resolve to owner emails
  // (following nested groups); every device with a matching owner is ringed.
  const groupNames = Object.keys(snapshot.groups ?? {});
  const groupKey = groupNames.join(",");
  useEffect(() => {
    if (highlightGroup && !groupKey.split(",").includes(highlightGroup))
      setHighlightGroup(null);
  }, [highlightGroup, groupKey, setHighlightGroup]);

  const inGroup = useMemo(() => {
    const set = new Set<number>();
    if (!result || !highlightGroup || !snapshot.groups) return set;
    const owners = resolveGroupOwners(snapshot.groups, highlightGroup);
    if (owners.size === 0) return set;
    for (const a of result.actors)
      if (a.user && owners.has(a.user.toLowerCase())) set.add(a.id);
    return set;
  }, [result, snapshot.groups, highlightGroup]);

  // The graph is edge-free at rest — the mesh only appears around the node (or
  // flow) the pointer is on, or the one that's pinned. Nothing focused ⇒ no
  // edges at all; ELK still lays the nodes out connectivity-aware from the
  // skeleton, they're just not drawn.
  const hasFocus = focusedActor != null || selectedPair != null;

  // The edges actually drawn — shared by the edge build and the node ports, so
  // an ingress / egress box appears exactly where a drawn arrow ends / starts.
  const vis = useMemo(() => {
    if (!model || !hasFocus) return [];
    return visibleEdges(model, {
      emphasize,
      focusedActor,
      selectedPair,
    }).filter((e) => !hidden.has(e.src) && !hidden.has(e.dst));
  }, [model, hasFocus, emphasize, focusedActor, selectedPair, hidden]);

  const ports = useMemo(() => portsOf(vis), [vis]);

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

  // Build RF nodes when layout or focus state changes.
  useEffect(() => {
    if (!model || !positioned || !layoutReady) return;
    setNodes(
      rfNodes(model, positioned, { focusedActor, connected, reach, hidden, inGroup, ports }),
    );
  }, [
    model,
    positioned,
    layoutReady,
    focusedActor,
    connected,
    reach,
    hidden,
    inGroup,
    ports,
    setNodes,
  ]);

  // Build RF edges from the style table whenever toggles / selection change.
  // Gated on `layoutReady` for the same reason as the node build — edges must
  // not reach RF before the nodes their endpoints reference are in place.
  useEffect(() => {
    if (!model || !layoutReady) return;
    if (!hasFocus) {
      setEdges([]);
      return;
    }
    const portDesc = anyProbe ? "any port" : probeLabel(activeProbe);
    setEdges(rfEdges(vis, model, result, portDesc));
  }, [
    model,
    layoutReady,
    hasFocus,
    vis,
    setEdges,
    result,
    anyProbe,
    activeProbe,
  ]);

  const presentKinds = useMemo(() => {
    const s = new Set<EdgeKind>();
    for (const e of edges) s.add((e.data as { kind: EdgeKind }).kind);
    return s;
  }, [edges]);

  useRefitOnSubject(activeLesson, result?.actors, layout, nodes.length);

  if (!result) return <PaneShell><p className="empty">Waiting for the engine…</p></PaneShell>;
  if (fatal)
    return (
      <PaneShell>
        <FatalNotice what="graph" />
      </PaneShell>
    );

  return (
    <GraphPanel
      badge={
        laying
          ? "laying out…"
          : anyProbe
            ? "reachability · any port"
            : `reachability · ${probeLabel(activeProbe)}`
      }
    >
      <UnhomedNotice />
      <ProbeBar probeNames={probeNames} />
      <FocusBar
        focusedLabel={focusedLabel}
        reach={reach}
        nodePinned={pinnedActor != null}
        hasPin={hasPin}
        groupNames={groupNames}
        inGroupCount={inGroup.size}
        onUnpin={() => {
          setFocusedActor(null);
          selectPair(null);
        }}
      />

      <div className={`graph-canvas${transientHover ? " node-focus" : ""}`}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          fitView
          fitViewOptions={{ padding: INITIAL_FIT_PADDING }}
          minZoom={0.2}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
          onNodeMouseEnter={(_, n) => {
            // Containers (group / extframe hulls) sit under their devices and
            // hold no hover — entering one must not cancel the clear the
            // device leave just queued.
            const a = actorIdOfNode(n);
            if (a != null) hover.hoverNode(a);
          }}
          onNodeMouseLeave={hover.scheduleClear}
          onEdgeMouseEnter={(_, e) => {
            hover.cancelClear();
            // During a bare node hover the node owns the view; otherwise
            // (pinned node / pinned pair / rest) an edge hover highlights it.
            if (transientHover) return;
            const p = pairOfEdgeId(e.id);
            if (p) hover.hoverEdge(p);
          }}
          onEdgeMouseLeave={hover.scheduleClear}
          onNodeClick={(_, n) => {
            const id = actorIdOfNode(n);
            if (id == null) return;
            selectPair(null);
            setFocusedActor(pinnedActor === id ? null : id);
          }}
          onEdgeClick={(_, e) => {
            const p = pairOfEdgeId(e.id);
            if (!p) return; // not a device↔device edge (route / tether / egress)
            setFocusedActor(null);
            selectPair(samePair(pinnedPair, p) ? null : p);
          }}
          onPaneClick={() => {
            hover.clear();
            setFocusedActor(null);
            selectPair(null);
          }}
        >
          <Background gap={16} color="var(--border)" />
          <Controls showInteractive={false} />
          {(frameRect || hulls.length > 0) && (
            <ViewportPortal>
              {frameRect && (
                <div
                  className="graph-tailnet-frame"
                  style={{
                    transform: `translate(${frameRect.x}px, ${frameRect.y}px)`,
                    width: frameRect.w,
                    height: frameRect.h,
                  }}
                >
                  <span className="graph-tailnet-frame-label">tailnet</span>
                </div>
              )}
              {hulls.length > 0 && (
                <svg className="graph-hull-layer" width="1" height="1" overflow="visible">
                  {hulls.map((h) => (
                    <g key={h.key}>
                      <path className="graph-hull" d={h.path} />
                      <text className="graph-hull-label" x={h.labelX} y={h.labelY}>
                        {h.label}
                      </text>
                    </g>
                  ))}
                </svg>
              )}
            </ViewportPortal>
          )}
        </ReactFlow>
      </div>

      <Legend present={presentKinds} directed={edges.length > 0} />

      <AnalysisStrip
        result={result}
        probe={activeProbe}
        sshMode={activeProbe.startsWith("ssh:")}
        selectedPair={selectedPair}
        onClosePair={() => selectPair(null)}
        onFocusDevice={setFocusedActor}
      />
    </GraphPanel>
  );
}

/** The graph's pane chrome. Its own component so usePaneMax runs on every
 *  render path, including the early returns before the engine is ready. */
function GraphPanel({ badge, children }: { badge?: string; children: React.ReactNode }) {
  return (
    <Panel
      id="impact"
      title="Connectivity graph"
      tabs={<ImpactTabs />}
      badge={badge}
      {...usePaneMax("impact")}
    >
      {children}
    </Panel>
  );
}

function PaneShell({ children }: { children: React.ReactNode }) {
  return (
    <GraphPanel>
      <div className="pane-body">{children}</div>
    </GraphPanel>
  );
}

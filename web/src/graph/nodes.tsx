import { memo } from "react";
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  Handle,
  Position,
  type EdgeProps,
  type NodeProps,
} from "@xyflow/react";

/**
 * What a node's connection port shows, derived from the edges currently drawn.
 * The left (ingress) port is where arrows end — the node *accepts* connections;
 * the right (egress) port is where they start — it *initiates* them. `both`
 * marks the end of a two-way edge (one curve, an arrowhead at each end).
 * `faded` = every edge on that port is de-emphasised.
 */
export interface PortState {
  mode: "in" | "out" | "both";
  faded: boolean;
}
export interface NodePorts {
  left?: PortState;
  right?: PortState;
}

const PORT_TITLE: Record<PortState["mode"], string> = {
  in: "ingress — accepts connections",
  out: "egress — initiates connections",
  both: "both ways — accepts and initiates",
};

function portClass(p: PortState | undefined, focused?: boolean): string | undefined {
  if (!p) return undefined;
  return `gport on ${p.mode}${p.faded ? " faded" : ""}${focused ? " focused" : ""}`;
}

/** The node's two React Flow handles, drawn as ingress / egress boxes when an
 *  arrow starts or ends there (hidden otherwise). */
function Ports({ ports, focused }: { ports?: NodePorts; focused?: boolean }) {
  return (
    <>
      <Handle
        type="target"
        position={Position.Left}
        className={portClass(ports?.left, focused)}
        aria-label={ports?.left ? PORT_TITLE[ports.left.mode] : undefined}
      />
      <Handle
        type="source"
        position={Position.Right}
        className={portClass(ports?.right, focused)}
        aria-label={ports?.right ? PORT_TITLE[ports.right.mode] : undefined}
      />
    </>
  );
}

interface DeviceNodeData {
  label: string;
  sub?: string;
  tagged: boolean;
  focused: boolean;
  dimmed: boolean;
  /** Owner resolves into the graph's highlighted `group:` (P2-6). */
  inGroup: boolean;
  /** Set on the focused node only: distinct peers it can reach / be reached by. */
  reach?: { out: number; in: number };
  ports?: NodePorts;
  [key: string]: unknown;
}

const DeviceNode = memo(function DeviceNode({ data }: NodeProps) {
  const d = data as DeviceNodeData;
  return (
    <div
      className={`gnode device${d.focused ? " focused" : ""}${d.dimmed ? " dimmed" : ""}${d.tagged ? " tagged" : ""}${d.inGroup ? " in-group" : ""}`}
    >
      <div className="gnode-label">{d.label}</div>
      {d.sub && <div className="gnode-sub">{d.sub}</div>}
      {d.reach && (
        <div
          className="gnode-reach"
          title={`It can start connections to ${d.reach.out} and accept them from ${d.reach.in}, on the selected port`}
        >
          can reach {d.reach.out} · reachable from {d.reach.in}
        </div>
      )}
      <Ports ports={d.ports} focused={d.focused} />
    </div>
  );
});

interface GroupNodeData {
  label: string;
  [key: string]: unknown;
}

// The ELK container that keeps a cluster's devices together during layout. It is
// invisible — the cluster is drawn as a hull region (P2-5, `hulls.ts`) — but the
// node still exists as the layout parent its device nodes are nested under.
const GroupNode = memo(function GroupNode() {
  return (
    <div className="gnode group">
      <Handle type="target" position={Position.Left} />
      <Handle type="source" position={Position.Right} />
    </div>
  );
});

// An external tailnet's frame (P2-3) — its shared nodes are ELK children of it,
// so it sits outside the local graph.
const ExtFrameNode = memo(function ExtFrameNode({ data }: NodeProps) {
  const d = data as GroupNodeData;
  return (
    <div className="gnode extframe">
      <Handle type="target" position={Position.Left} />
      <div className="gnode-extframe-label">{d.label} · external tailnet</div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
});

const InternetNode = memo(function InternetNode({ data }: NodeProps) {
  const d = data as { dimmed?: boolean; ports?: NodePorts };
  return (
    <div className={`gnode internet${d.dimmed ? " dimmed" : ""}`}>
      <div className="gnode-label">internet</div>
      <Ports ports={d.ports} />
    </div>
  );
});

interface TargetNodeData {
  label: string;
  sub?: string;
  variant: "route" | "service" | "external";
  dimmed: boolean;
  ports?: NodePorts;
  [key: string]: unknown;
}

const TARGET_GLYPH: Record<string, string> = {
  route: "⇥",
  service: "◈",
  external: "⇄",
};

const TargetNode = memo(function TargetNode({ data }: NodeProps) {
  const d = data as TargetNodeData;
  return (
    <div className={`gnode target ${d.variant}${d.dimmed ? " dimmed" : ""}`}>
      <div className="gnode-label">
        <span className="gnode-glyph">{TARGET_GLYPH[d.variant]}</span> {d.label}
      </div>
      {d.sub && <div className="gnode-sub">{d.sub}</div>}
      <Ports ports={d.ports} />
    </div>
  );
});

export const nodeTypes = {
  device: DeviceNode,
  group: GroupNode,
  extframe: ExtFrameNode,
  internet: InternetNode,
  target: TargetNode,
};

/** Which ends of an edge get an arrowhead. Direction = who opens the
 *  connection (replies ride the same connection back). */
export type EdgeArrows = "none" | "end" | "both";

/**
 * A bezier edge that adds a native hover tooltip (`data.tip`), arrowheads
 * (`data.arrows`), and renders its own HTML label — the default edge can't
 * carry the `<title>`, and once we use a custom edge we lose the built-in label
 * renderer too.
 */
function TipEdge(props: EdgeProps) {
  const { id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, style, label } =
    props;
  const [path, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });
  const tip = typeof props.data?.tip === "string" ? (props.data.tip as string) : "";
  const arrows = (props.data?.arrows as EdgeArrows | undefined) ?? "none";
  // One marker per edge so the head takes the edge's own (CSS-var) colour —
  // React Flow's shared markers want a literal colour string. Opacity needs no
  // copy: the path's `opacity` composites its markers along with it.
  const markerId = `arrow-${id}`;
  const markerUrl = `url('#${markerId}')`;
  return (
    <>
      {tip ? <title>{tip}</title> : null}
      {arrows !== "none" && (
        <defs>
          <marker
            id={markerId}
            viewBox="0 0 10 10"
            refX="9"
            refY="5"
            markerWidth="12"
            markerHeight="12"
            markerUnits="userSpaceOnUse"
            orient="auto-start-reverse"
          >
            <path d="M0,1 L10,5 L0,9 z" style={{ fill: style?.stroke }} />
          </marker>
        </defs>
      )}
      <BaseEdge
        id={id}
        path={path}
        style={style}
        markerEnd={arrows !== "none" ? markerUrl : undefined}
        markerStart={arrows === "both" ? markerUrl : undefined}
      />
      {label ? (
        <EdgeLabelRenderer>
          <div
            className="gedge-label"
            title={tip || undefined}
            style={{
              position: "absolute",
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            }}
          >
            {label}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}

export const edgeTypes = { tip: TipEdge };

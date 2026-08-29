import { describe, expect, it } from "vitest";
import type { EngineResult, Verdict } from "../engine/types";
import { buildGraphModel, pairOfEdgeId } from "./model";
import { reachCounts, visibleEdges } from "./visibility";

// alice-laptop (1) and bob-laptop (2) are user-owned; web (3) and db-1 (4) are
// tagged. Allowed: 1↔3 both ways, 2→3, 3→4. Every other pair is denied.
const ALLOW = new Set(["1->3", "3->1", "2->3", "3->4"]);
const ACTORS: EngineResult["actors"] = [
  { id: 1, name: "alice-laptop", kind: "user", user: "alice@example.com", ips: [] },
  { id: 2, name: "bob-laptop", kind: "user", user: "bob@example.com", ips: [] },
  { id: 3, name: "web", kind: "tagged", tags: ["tag:web"], ips: [] },
  { id: 4, name: "db-1", kind: "tagged", tags: ["tag:db"], ips: [] },
];

function result(): EngineResult {
  const cells: Record<string, Verdict> = {};
  for (const s of ACTORS)
    for (const d of ACTORS)
      if (s.id !== d.id) cells[`${s.id}->${d.id}`] = ALLOW.has(`${s.id}->${d.id}`) ? "allow" : "deny";
  return {
    matrices: { "tcp/443": cells },
    actors: ACTORS,
    engine: {} as EngineResult["engine"],
  };
}

const model = () => buildGraphModel(result(), "user", "tcp/443");
const focusOn = (focusedActor: number | null, selectedPair: { src: number; dst: number } | null = null) =>
  visibleEdges(model(), { emphasize: false, focusedActor, selectedPair });
const ids = (es: { id: string }[]) => es.map((e) => e.id);

describe("buildGraphModel", () => {
  it("draws one edge per ordered pair, and lays out only from allowed ones", () => {
    const m = model();
    expect(m.edges).toHaveLength(12);
    expect(new Set(ids(m.edges)).size).toBe(12);
    expect(ids(m.skeleton).sort()).toEqual(["e:1-3", "e:2-3", "e:3-1", "e:3-4"]);
  });

  it("classifies allowed pairs by shared cluster and denied pairs as deny", () => {
    const kind = (id: string) => model().edges.find((e) => e.id === id)?.kind;
    expect(kind("e:1-3")).toBe("boundary");
    expect(kind("e:1-2")).toBe("deny");
  });
});

describe("pairOfEdgeId", () => {
  it("reads back the pair of every device→device edge, and nothing else", () => {
    for (const e of model().edges) expect(pairOfEdgeId(e.id)).toEqual({ src: e.src, dst: e.dst });
    expect(pairOfEdgeId("e:1-net")).toBeNull();
    expect(pairOfEdgeId("re:1:tgt:svc:web")).toBeNull();
    expect(pairOfEdgeId("te:dev:1:tgt:svc:web")).toBeNull();
  });
});

describe("visibleEdges", () => {
  it("never repeats an edge id, whichever node is focused", () => {
    // Regression: when mergeReverse kept the reverse half of a pair it emitted
    // that half twice, and the duplicate RF edge stayed drawn after unhover.
    for (const a of ACTORS) {
      const out = ids(focusOn(a.id));
      expect(new Set(out).size, `focus ${a.name}: ${out}`).toBe(out.length);
    }
  });

  it("shows a focused node's whole neighbourhood, denies included", () => {
    // 3→4 allowed but 4→3 denied: different kinds, so not merged.
    expect(ids(focusOn(4)).sort()).toEqual(["e:3-4", "e:4-1", "e:4-2", "e:4-3"]);
  });

  it("merges a same-kind two-way pair, keeping the half that leaves the focus", () => {
    const fromAlice = focusOn(1).find((e) => e.kind === "boundary");
    expect(fromAlice).toMatchObject({ id: "e:1-3", bidir: true });
    const fromWeb = focusOn(3).find((e) => e.bidir && e.kind === "boundary");
    expect(fromWeb?.id).toBe("e:3-1");
  });

  it("highlights only the exact selected flow and fades the rest", () => {
    const out = focusOn(null, { src: 2, dst: 3 });
    const sel = out.find((e) => e.id === "e:2-3");
    expect(sel).toMatchObject({ highlighted: true, faded: false });
    expect(out.filter((e) => e.id !== "e:2-3").every((e) => e.faded)).toBe(true);
  });

  it("with nothing focused, shows every allowed edge and no denies", () => {
    const out = focusOn(null);
    expect(out.some((e) => e.kind === "deny")).toBe(false);
    expect(ids(out).sort()).toEqual(["e:1-3", "e:2-3", "e:3-4"]);
  });
});

describe("reachCounts", () => {
  it("counts distinct allowed peers each way", () => {
    expect(reachCounts(model(), 3)).toEqual({ out: 2, in: 2 });
    expect(reachCounts(model(), 4)).toEqual({ out: 0, in: 1 });
  });
});

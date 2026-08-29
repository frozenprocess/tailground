import { describe, expect, it } from "vitest";
import type { EngineResult } from "../engine/types";
import { buildGraphModel } from "./model";
import { visibleEdges } from "./visibility";
import { portsOf, rfEdges } from "./rfElements";

const result: EngineResult = {
  matrices: { "tcp/443": { "1->2": "allow", "2->1": "allow", "1->3": "deny", "3->1": "allow", "2->3": "deny", "3->2": "deny" } },
  grants: { "1->2": ["example.com/cap/web"] },
  actors: [
    { id: 1, name: "alice-laptop", kind: "user", user: "alice@example.com", ips: [] },
    { id: 2, name: "web", kind: "tagged", tags: ["tag:web"], ips: [] },
    { id: 3, name: "db-1", kind: "tagged", tags: ["tag:db"], ips: [] },
  ],
  engine: {} as EngineResult["engine"],
};
const model = buildGraphModel(result, "user", "tcp/443");
const vis = visibleEdges(model, { emphasize: false, focusedActor: 1, selectedPair: null });

describe("rfEdges", () => {
  const edges = rfEdges(vis, model, result, "tcp:443");
  const byId = Object.fromEntries(edges.map((e) => [e.id, e]));

  it("names both directions of a two-way edge, with its grants", () => {
    expect(byId["e:1-2"].data).toMatchObject({
      kind: "boundary",
      arrows: "both",
      tip: "alice-laptop → web · allow · tcp:443 · grant: example.com/cap/web\nweb → alice-laptop · allow · tcp:443",
    });
  });

  it("dashes denies, keeps them unselectable, and says the port", () => {
    expect(byId["e:1-3"]).toMatchObject({
      selectable: false,
      style: { strokeDasharray: "5 4", opacity: 0.85 },
      data: { kind: "deny", arrows: "end", tip: "alice-laptop → db-1 · denied on tcp:443" },
    });
  });
});

describe("portsOf", () => {
  it("marks where arrows start (right) and end (left)", () => {
    const ports = portsOf(vis);
    // 1↔2 is drawn from alice's side, so it lights web's ingress as `both`;
    // alice's ingress only carries the one-way 3→1.
    expect(ports.get("dev:1")).toEqual({
      right: { mode: "both", faded: false },
      left: { mode: "in", faded: false },
    });
    expect(ports.get("dev:2")).toEqual({ left: { mode: "both", faded: false } });
    expect(ports.get("dev:3")).toEqual({ left: { mode: "in", faded: false }, right: { mode: "out", faded: false } });
  });
});

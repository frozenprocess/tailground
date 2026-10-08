// @vitest-environment jsdom
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { deflateSync, strToU8 } from "fflate";
import { encodeShare } from "./shareCodec";
import { SNAPSHOT_PRESETS } from "../state/presets";
import { defaultProbes } from "../engine/types";
import fixtures from "./testdata/shares.json";
import { buildLongShareURL, buildShareURL, decodeShare, fetchShareToken, isOwnShare, shareIdFromHash, shareRouteFromHash, shareTokenFromHash, type SharedState } from "./share";

beforeEach(() => {
  localStorage.clear();
  window.history.replaceState(null, "", "/play/");
});
afterEach(() => vi.unstubAllGlobals());

function rawToken(tuple: unknown): string {
  const bytes = deflateSync(strToU8(JSON.stringify(tuple)));
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

describe("share wire compatibility", () => {
  for (const fixture of fixtures) {
    it(`preserves the pre-refactor ${fixture.name} token`, () => {
      expect(decodeShare(fixture.token)).toEqual(fixture.decoded);
      expect(buildLongShareURL(fixture.state as SharedState).split("#s=")[1]).toBe(fixture.token);
    });
  }
  it.each(["", "!invalid!", "e30", "W10"])("rejects malformed token %s", (token) => {
    expect(decodeShare(token)).toBeNull();
  });

  it.each([
    ["snapshot without arrays", { users: 1, devices: "x" }],
    ["device with non-string name", { users: [], devices: [{ id: 1, name: { a: 1 } }] }],
    ["device tags not an array", { users: [], devices: [{ id: 1, name: "a", tags: "tag:x" }] }],
    ["groups of wrong shape", { users: [], devices: [], groups: { "group:a": "bob" } }],
    ["snapshot is an array", []],
    ["snapshot is a string", "x"],
  ])("rejects a hostile snapshot: %s", (_n, snapshot) => {
    expect(decodeShare(rawToken(["policy", snapshot]))).toBeNull();
  });

  it.each([
    ["probes not an array", ["p", { users: [], devices: [] }, { a: 1 }]],
    ["probe with bad port", ["p", { users: [], devices: [] }, [{ name: "x", proto: "tcp", port: "80" }]]],
    ["sshUsers not strings", ["p", { users: [], devices: [] }, 0, [1, 2]]],
    ["activeProbe not a string", ["p", { users: [], devices: [] }, 0, 0, 0, { x: 1 }]],
    ["lesson not a string", ["p", { users: [], devices: [] }, 0, 0, 0, "https", 0, { x: 1 }]],
  ])("rejects a hostile payload: %s", (_n, tuple) => {
    expect(decodeShare(rawToken(tuple))).toBeNull();
  });

  it("accepts every shipped snapshot preset", () => {
    for (const preset of SNAPSHOT_PRESETS) {
      const token = encodeShare({ policy: "{}", snapshot: preset.snapshot, probes: defaultProbes() });
      expect(decodeShare(token), preset.id).not.toBeNull();
    }
  });

  it("accepts a minimal well-formed payload", () => {
    expect(decodeShare(rawToken(["p", { users: [], devices: [] }]))?.policy).toBe("p");
  });

  it("rejects a token that inflates past the size cap", () => {
    // Valid payload shape, ~10 MiB once inflated, a few KB on the wire.
    const bomb = strToU8(JSON.stringify(["a".repeat(10 << 20), {}]));
    const bytes = deflateSync(bomb, { level: 9 });
    let bin = "";
    for (const b of bytes) bin += String.fromCharCode(b);
    const token = btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    expect(token.length).toBeLessThan(64 << 10);
    expect(decodeShare(token)).toBeNull();
  });
});

const state = fixtures[0].state as SharedState;
it("stores own short-link IDs and posts the exact compressed token", async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ id: "Abc12345" }) });
  vi.stubGlobal("fetch", fetch);
  expect(await buildShareURL(state)).toBe(`${location.origin}/play/#/s/Abc12345`);
  expect(fetch).toHaveBeenCalledWith("/api/share", expect.objectContaining({ method: "POST", body: fixtures[0].token }));
  expect(isOwnShare("Abc12345")).toBe(true);
  expect(isOwnShare("Other123")).toBe(false);
});
it.each([undefined, "short", "bad/id!!", "12345678901234567"])("falls back for invalid response ID %s", async (id) => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ id }) }));
  expect(await buildShareURL(state)).toBe(buildLongShareURL(state));
});
it.each(["reject", "http", "json"])("falls back on %s failure", async (failure) => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async () => {
    if (failure === "reject") throw new Error("offline");
    return { ok: failure !== "http", json: async () => { throw new Error("invalid JSON"); } };
  }));
  expect(await buildShareURL(state)).toBe(buildLongShareURL(state));
});
it("fetches and trims stored tokens, handling missing and failed requests", async () => {
  const fetch = vi.fn().mockResolvedValueOnce({ ok: true, text: async () => "  token\n" }).mockResolvedValueOnce({ ok: false }).mockRejectedValueOnce(new Error("offline"));
  vi.stubGlobal("fetch", fetch);
  expect(await fetchShareToken("Abc12345")).toBe("token");
  expect(await fetchShareToken("Abc12345")).toBeNull();
  expect(await fetchShareToken("Abc12345")).toBeNull();
  expect(fetch).toHaveBeenCalledWith("/api/share/Abc12345", expect.anything());
});
it("parses both canonical and route-attached tokens", () => {
  expect(shareIdFromHash("#/s/Abc12345/")).toBe("Abc12345");
  expect(shareIdFromHash("#/s/short")).toBeNull();
  expect(shareTokenFromHash("#s=abc_-123")).toBe("abc_-123");
  expect(shareRouteFromHash("#s=abc")).toBeNull();
  expect(shareTokenFromHash("#/graph/#s=abc")).toBe("abc");
  expect(shareRouteFromHash("#/graph/#s=abc")).toBe("#/graph");
});

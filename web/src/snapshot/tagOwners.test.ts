import { parse as parseJsonc } from "jsonc-parser";
import { describe, expect, it } from "vitest";
import { addTagOwners, undefinedTags } from "./tagOwners";

describe("tagOwners", () => {
  const devices = [
    { id: 1, name: "a", tags: ["tag:app", "tag:new"] },
    { id: 2, name: "b", tags: ["tag:new", "tag:other"] },
    { id: 3, name: "c" },
  ];
  const policy = `{
  // keep me
  "tagOwners": {
    "tag:app": ["group:eng"], // and me
  },
  "grants": [],
}`;

  it("finds tags the policy doesn't define, once each", () => {
    expect(undefinedTags(policy, devices)).toEqual({ tags: ["tag:new", "tag:other"], editable: true });
  });
  it("treats a policy with no tagOwners as defining nothing", () => {
    expect(undefinedTags("{}", devices).tags).toEqual(["tag:app", "tag:new", "tag:other"]);
  });
  it("reports an unparseable policy as not editable", () => {
    const all = ["tag:app", "tag:new", "tag:other"];
    expect(undefinedTags("{ nope", devices)).toEqual({ tags: all, editable: false });
    expect(undefinedTags("[1]", devices)).toEqual({ tags: all, editable: false });
  });

  it("adds entries, keeping comments, trailing commas and existing owners", () => {
    const out = addTagOwners(policy, ["tag:new", "tag:other"], "me@example.com");
    expect(out).toContain("// keep me");
    // the existing entry, with its comment, is byte-for-byte unchanged
    expect(out).toContain('    "tag:app": ["group:eng"], // and me\n');
    expect(out).toContain('    "tag:new": ["me@example.com"],\n    "tag:other": ["me@example.com"],\n    "tag:app"');
    expect(undefinedTags(out, devices).tags).toEqual([]);
    const owners = parseJson(out).tagOwners;
    expect(owners["tag:app"]).toEqual(["group:eng"]);
    expect(owners["tag:new"]).toEqual(["me@example.com"]);
    expect(owners["tag:other"]).toEqual(["me@example.com"]);
  });
  it("creates tagOwners when absent, and uses an empty owner list without an owner", () => {
    const out = addTagOwners('{\n  "grants": []\n}', ["tag:x"]);
    expect(parseJson(out)).toEqual({ grants: [], tagOwners: { "tag:x": [] } });
    expect(out).toBe('{\n  "tagOwners": {\n    "tag:x": [],\n  },\n  "grants": []\n}');
  });
  it("handles empty objects and unusual policies", () => {
    expect(addTagOwners("{}", ["tag:x"], "a@b.c")).toBe('{\n  "tagOwners": {\n    "tag:x": ["a@b.c"]\n  }\n}');
    expect(addTagOwners('{ "tagOwners": {} }', ["tag:x", "tag:y"], "a@b.c")).toBe(
      '{ "tagOwners": {\n  "tag:x": ["a@b.c"],\n  "tag:y": ["a@b.c"]\n} }',
    );
    expect(addTagOwners('{"tagOwners": 5}', ["tag:x"])).toBe('{"tagOwners": 5}');
    expect(addTagOwners("{ nope", ["tag:x"])).toBe("{ nope");
    expect(addTagOwners(policy, [], "a@b.c")).toBe(policy);
  });
  it("matches the policy's own indentation", () => {
    const tabbed = '{\n\t"grants": []\n}';
    expect(addTagOwners(tabbed, ["tag:x"])).toBe('{\n\t"tagOwners": {\n\t\t"tag:x": [],\n\t},\n\t"grants": []\n}');
  });
});

function parseJson(text: string) {
  return parseJsonc(text, [], { allowTrailingComma: true }) as Record<string, any>;
}

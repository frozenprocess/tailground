import { parse, parseTree, type Node, type ParseError } from "jsonc-parser";
import type { Device } from "../engine/types";
import { isObj } from "./json";

// A tag on a device that no tagOwners entry defines is a policy error (the engine
// flags the device), so an imported tagged device would arrive already "broken".
// These helpers find those tags and add entries for them, editing the policy text
// in place so comments and formatting survive.

const JSONC = { allowTrailingComma: true, disallowComments: false } as const;

/**
 * Tags used by `devices` that the policy's tagOwners doesn't define, in first-seen
 * order. `editable` is false when the policy can't be parsed as an object, in
 * which case every tag is reported and nothing can be added safely.
 */
export function undefinedTags(policy: string, devices: Device[]): { tags: string[]; editable: boolean } {
  const errors: ParseError[] = [];
  const root: unknown = parse(policy, errors, JSONC);
  const editable = errors.length === 0 && isObj(root);
  // If the policy can't be read we can't tell what's defined: report every tag.
  const defined = editable && isObj(root.tagOwners) ? root.tagOwners : {};
  const tags: string[] = [];
  for (const d of devices) {
    for (const t of d.tags ?? []) if (!(t in defined) && !tags.includes(t)) tags.push(t);
  }
  return { tags, editable };
}

/** Leading whitespace of the line containing `offset`. */
function lineIndent(text: string, offset: number): string {
  const start = text.lastIndexOf("\n", offset - 1) + 1;
  return /^[ \t]*/.exec(text.slice(start, offset))![0];
}

/**
 * Add `"tag:x": [owner]` to the policy's tagOwners for each tag (creating the
 * tagOwners object if needed). With no owner the entry is an empty list, which
 * Tailscale reads as "admins only".
 *
 * The text is edited in place, not re-serialized: new entries go at the start of
 * tagOwners, indented like the surrounding lines, so existing entries and their
 * comments are never touched. If the policy can't be parsed as an object, or
 * tagOwners isn't an object, it is returned unchanged.
 */
export function addTagOwners(policy: string, tags: string[], owner?: string): string {
  if (tags.length === 0) return policy;
  const errors: ParseError[] = [];
  const root = parseTree(policy, errors, JSONC);
  if (errors.length > 0 || !root || root.type !== "object") return policy;

  const value = JSON.stringify(owner ? [owner] : []);
  const entry = (tag: string) => `${JSON.stringify(tag)}: ${value}`;
  const rootIndent = lineIndent(policy, root.offset);
  const firstProp = root.children?.[0];
  const unit = (firstProp && lineIndent(policy, firstProp.offset).slice(rootIndent.length)) || "  ";

  const prop = root.children?.find((p) => p.children?.[0]?.value === "tagOwners");
  const owners: Node | undefined = prop?.children?.[1];
  const isBlank = (n: Node) => policy.slice(n.offset + 1, n.offset + n.length - 1).trim() === "";
  const splice = (from: number, to: number, text: string) => policy.slice(0, from) + text + policy.slice(to);

  if (prop && owners?.type !== "object") return policy; // something we don't understand

  if (owners) {
    const ind = lineIndent(policy, owners.offset) + unit;
    if (owners.children?.length || !isBlank(owners)) {
      // Entries (or comments) already there: put ours first, each with a trailing comma.
      return splice(owners.offset + 1, owners.offset + 1, tags.map((t) => `\n${ind}${entry(t)},`).join(""));
    }
    const close = lineIndent(policy, owners.offset);
    const body = tags.map((t) => `${ind}${entry(t)}`).join(",\n");
    return splice(owners.offset, owners.offset + owners.length, `{\n${body}\n${close}}`);
  }

  // No tagOwners yet.
  const ind = rootIndent + unit;
  if (root.children?.length || !isBlank(root)) {
    const body = tags.map((t) => `${ind}${unit}${entry(t)},`).join("\n");
    return splice(root.offset + 1, root.offset + 1, `\n${ind}"tagOwners": {\n${body}\n${ind}},`);
  }
  const body = tags.map((t) => `${ind}${unit}${entry(t)}`).join(",\n");
  return splice(root.offset, root.offset + root.length, `{\n${ind}"tagOwners": {\n${body}\n${ind}}\n${rootIndent}}`);
}

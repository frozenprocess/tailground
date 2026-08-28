import schema from "../schema/policy.schema.json";

/** Top-level keys the policy file accepts, from the hand-maintained schema. */
export const TOP_LEVEL_KEYS: string[] = Object.keys(
  (schema as { properties: Record<string, unknown> }).properties,
);

function at(path: string[]): any {
  let node: any = schema;
  for (const step of path) node = node?.[step];
  return node;
}

function enumOf(path: string[]): string[] {
  const node = at(path);
  return Array.isArray(node?.enum) ? node.enum : [];
}

export const ACL_ACTIONS = enumOf(["$defs", "acl", "properties", "action"]);
export const SSH_ACTIONS = enumOf(["$defs", "ssh", "properties", "action"]);

/** description for a top-level key, or a $def's property. */
export function describe(key: string): string | undefined {
  const top = at(["properties", key]);
  if (top?.description) return top.description;
  return undefined;
}

/** property name → description for the object schema at $defs.<def>. */
export function defProps(def: string): Record<string, string> {
  const props = at(["$defs", def, "properties"]) ?? {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(props)) {
    out[k] = (v as { description?: string }).description ?? "";
  }
  return out;
}

/** The autogroup tokens the engine models . */
export const SUPPORTED_AUTOGROUPS = [
  "internet",
  "member",
  "tagged",
  "self",
  "nonroot",
  "danger-all",
];

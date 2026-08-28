import {
  autocompletion,
  snippetCompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from "@codemirror/autocomplete";
import { hoverTooltip } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { getLocation, parse as parseJsonc } from "jsonc-parser";
import {
  ACL_ACTIONS,
  SSH_ACTIONS,
  SUPPORTED_AUTOGROUPS,
  TOP_LEVEL_KEYS,
  defProps,
  describe,
} from "./schema";

// Snippet templates for the top-level keys (CM ${} placeholder syntax) so
// completion inserts a useful starting shape.
const TOP_LEVEL_SNIPPETS: Record<string, string> = {
  acls: '"acls": [\n  { "action": "accept", "src": ["${*}"], "dst": ["${*:*}"] }\n]',
  grants: '"grants": [\n  { "src": ["${*}"], "dst": ["${*}"], "ip": ["${tcp:443}"] }\n]',
  groups: '"groups": {\n  "group:${team}": ["${user@example.com}"]\n}',
  hosts: '"hosts": {\n  "${name}": "${100.64.0.0/32}"\n}',
  tagOwners: '"tagOwners": {\n  "tag:${web}": ["${user@example.com}"]\n}',
  ssh: '"ssh": [\n  { "action": "check", "src": ["${*}"], "dst": ["${*}"], "users": ["autogroup:nonroot"] }\n]',
  nodeAttrs: '"nodeAttrs": [\n  { "target": ["${*}"], "attr": ["${mullvad}"] }\n]',
  autoApprovers: '"autoApprovers": {\n  "routes": { "${10.0.0.0/24}": ["tag:${router}"] }\n}',
  postures: '"postures": {\n  "posture:${corp}": ["node:os == \'macos\'"]\n}',
  ipsets: '"ipsets": {\n  "ipset:${corp}": ["${10.0.0.0/8}"]\n}',
  attrConfig: '"attrConfig": {\n  "custom:${env}": { "type": "${string}" }\n}',
  externalTailnets:
    '"externalTailnets": {\n  "${partner}": { "externalID": "${id}", "allowIncomingConnections": true }\n}',
  tests: '"tests": [\n  { "src": "${*}", "accept": ["${tag:web:443}"], "deny": [] }\n]',
  sshTests: '"sshTests": [\n  { "src": "${*}", "dst": ["${tag:web}"], "accept": ["ubuntu"] }\n]',
  randomizeClientPort: '"randomizeClientPort": true',
};

const RULE_ARRAYS = ["acls", "grants", "ssh", "nodeAttrs", "tests", "sshTests"] as const;
const ARRAY_DEF: Record<string, string> = {
  acls: "acl",
  grants: "grant",
  ssh: "ssh",
  nodeAttrs: "nodeAttr",
  tests: "test",
  sshTests: "sshTest",
};

function options(
  list: { label: string; detail?: string; info?: string; apply?: string; snippet?: string }[],
): Completion[] {
  return list.map((o) =>
    o.snippet
      ? snippetCompletion(o.snippet, {
          label: o.label,
          detail: o.detail,
          info: o.info,
          type: "property",
        })
      : {
          label: o.label,
          detail: o.detail,
          info: o.info,
          apply: o.apply,
          type: "property",
        },
  );
}

// externalAliases best-effort reads the aliases declared in the doc's
// `externalTailnets` section, for group:// / tag:// completion.
function externalAliases(text: string): string[] {
  try {
    const doc = parseJsonc(text) as Record<string, unknown> | undefined;
    const et = doc?.externalTailnets;
    if (et && typeof et === "object" && !Array.isArray(et)) {
      return Object.keys(et as Record<string, unknown>);
    }
  } catch {
    /* editor mid-edit — ignore */
  }
  return [];
}

function source(ctx: CompletionContext): CompletionResult | null {
  const text = ctx.state.doc.toString();
  const loc = getLocation(text, ctx.pos);
  const path = loc.path;
  const word = ctx.matchBefore(/[\w:@.\-*/]*/);
  const from = word ? word.from : ctx.pos;

  // Top-level keys.
  if (loc.isAtPropertyKey && path.length <= 1) {
    return {
      from,
      options: options(
        TOP_LEVEL_KEYS.map((k) => ({
          label: k,
          detail: "policy key",
          info: describe(k),
          snippet: TOP_LEVEL_SNIPPETS[k],
          apply: TOP_LEVEL_SNIPPETS[k] ? undefined : `"${k}": `,
        })),
      ),
    };
  }

  // Keys inside a rule-array element, e.g. path = ["acls", 0].
  if (
    loc.isAtPropertyKey &&
    path.length === 2 &&
    typeof path[0] === "string" &&
    RULE_ARRAYS.includes(path[0] as (typeof RULE_ARRAYS)[number]) &&
    typeof path[1] === "number"
  ) {
    const props = defProps(ARRAY_DEF[path[0] as string]);
    return {
      from,
      options: options(
        Object.entries(props).map(([k, info]) => ({
          label: k,
          detail: "field",
          info,
          apply: `"${k}": `,
        })),
      ),
    };
  }

  // Enum values.
  if (!loc.isAtPropertyKey && path[path.length - 1] === "action") {
    const enums = path[0] === "ssh" ? SSH_ACTIONS : ACL_ACTIONS;
    return { from, options: options(enums.map((e) => ({ label: e, detail: "action" }))) };
  }

  // Alias values in src / dst / target arrays.
  const key = path[path.length - 2];
  if (!loc.isAtPropertyKey && (key === "src" || key === "dst" || key === "target")) {
    const ag = SUPPORTED_AUTOGROUPS.map((a) => ({
      label: `autogroup:${a}`,
      detail: "autogroup",
    }));
    const aliases = externalAliases(text);
    // External references are only valid in a grant's src (Tailscale rejects
    // them in acls, ssh and any dst): access to another tailnet's nodes is
    // granted by that tailnet's policy.
    const extRefs =
      key === "src" && path[0] === "grants"
        ? aliases.length > 0
          ? aliases.flatMap((a) => [
              { label: `group://${a}/`, detail: "external tailnet group" },
              { label: `tag://${a}/`, detail: "external tailnet tag" },
            ])
          : [
              { label: "group://", detail: "cross-tailnet group reference" },
              { label: "tag://", detail: "cross-tailnet tag reference" },
            ]
        : [];
    return {
      from,
      options: options([
        { label: "*", detail: "everything" },
        { label: "tag:", detail: "a tag" },
        { label: "group:", detail: "a group" },
        ...(key === "dst" ? [{ label: "svc:", detail: "a Tailscale Service" }] : []),
        ...extRefs,
        ...ag,
      ]),
    };
  }

  return null;
}

const keyHover = hoverTooltip((view, pos) => {
  const text = view.state.doc.toString();
  const loc = getLocation(text, pos);
  const key = loc.path[loc.path.length - 1];
  if (typeof key !== "string") return null;

  let info = describe(key);
  if (!info && loc.path.length >= 2 && typeof loc.path[0] === "string") {
    const def = ARRAY_DEF[loc.path[0]];
    if (def) info = defProps(def)[key];
  }
  if (!info) return null;

  const line = view.state.doc.lineAt(pos);
  return {
    pos: line.from,
    end: line.to,
    above: true,
    create() {
      const dom = document.createElement("div");
      dom.className = "cm-schema-hover";
      dom.textContent = info!;
      return { dom };
    },
  };
});

export function policyCompletion(): Extension {
  return [
    autocompletion({ override: [source], activateOnTyping: true }),
    keyHover,
  ];
}

import { createPortal } from "react-dom";
import { useEffect, useMemo, useRef, useState, type ChangeEvent } from "react";
import { useStore } from "../state/store";
import type { Snapshot } from "../engine/types";
import {
  addTagOwners,
  applyImport,
  serviceKey,
  ImportError,
  parseTailnetImport,
  undefinedTags,
  type ImportResult,
} from "./importTailnet";

type Method = "api" | "cli";

// The OAuth secret is read with `read -s` so it never lands in shell history, and
// it never touches this page: the user runs this in their own terminal, which
// saves the device list to a file (no secret in it) for them to paste back.
const API_COMMAND = `read -rsp 'OAuth client secret: ' TS_SECRET; echo
TOKEN=$(curl -s https://api.tailscale.com/api/v2/oauth/token \\
  -d client_id=YOUR_CLIENT_ID \\
  -d client_secret="$TS_SECRET" \\
  -d grant_type=client_credentials | jq -r .access_token)
get() { curl -s -H "Authorization: Bearer $TOKEN" "https://api.tailscale.com/api/v2/tailnet/-/$1"; }
{ get "devices?fields=all"; get users; get services; } | jq -s add > tailnet.json
unset TS_SECRET TOKEN
echo "wrote $(pwd)/tailnet.json"`;

const CLI_COMMAND = "tailscale status --json";

/** Snapshot-toolbar button that opens the "import from your tailnet" dialog. */
export function ImportTailnetButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        className="btn import-btn"
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
        title="Replace the devices with the ones from your real tailnet (pasted, parsed in your browser)"
      >
        Import Your tailnet
      </button>
      {open && <ImportModal onClose={() => setOpen(false)} />}
    </>
  );
}

function ImportModal({ onClose }: { onClose: () => void }) {
  const snapshot = useStore((s) => s.snapshot);
  const setSnapshot = useStore((s) => s.setSnapshot);
  const policy = useStore((s) => s.policy);
  const setPolicy = useStore((s) => s.setPolicy);
  const [addTags, setAddTags] = useState(true);
  const [method, setMethod] = useState<Method>("api");
  const [text, setText] = useState("");
  const taRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    taRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const parsed = useMemo<{ ok: ImportResult } | { err: string } | null>(() => {
    if (!text.trim()) return null;
    try {
      return { ok: parseTailnetImport(text) };
    } catch (e) {
      return { err: e instanceof ImportError ? e.message : "Couldn't read that." };
    }
  }, [text]);
  const result = parsed && "ok" in parsed ? parsed.ok : null;

  const command = method === "api" ? API_COMMAND : CLI_COMMAND;

  // Read a chosen file here, in the browser; it is never uploaded.
  const onFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ""; // allow re-choosing the same file
    if (file) setText(await file.text());
  };
  // Tags the policy doesn't define would flag every tagged device as a problem.
  const tagPlan = useMemo(() => (result ? undefinedTags(policy, result.devices) : null), [policy, result]);
  const tagOwner = result?.owners[0];

  const apply = () => {
    if (!result) return;
    setSnapshot(applyImport(snapshot, result));
    if (addTags && tagPlan && tagPlan.tags.length > 0) {
      setPolicy(addTagOwners(policy, tagPlan.tags, tagOwner));
    }
    onClose();
  };

  return createPortal(
    <div
      className="consent-backdrop"
      role="presentation"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="consent-card import-modal" role="dialog" aria-modal="true" aria-label="Import from your tailnet">
        <h2>Import from your tailnet</h2>
        <p>
          Run a command in your own terminal and bring its result back here (paste it or choose the saved
          file). Tailground turns it into the
          snapshot's devices. <strong>Nothing you paste is sent anywhere</strong> — it is parsed in this
          tab, and your credentials never enter this page.
        </p>

        <MethodTabs method={method} onChange={setMethod} />

        {method === "api" ? (
          <p className="import-hint">
            Needs an <a href="https://login.tailscale.com/admin/settings/oauth" target="_blank" rel="noopener noreferrer">OAuth client</a> with
            only the read-only <code>devices:core:read</code>, <code>users:read</code> and{" "}
            <code>services:read</code> scopes (and <code>jq</code>). Without the last two you still get
            the devices, just no roles or services. Replace{" "}
            <code>YOUR_CLIENT_ID</code>; the secret is prompted for. The device list is saved to{" "}
            <code>tailnet.json</code> (devices, users and services; no credentials).
          </p>
        ) : (
          <p className="import-hint">
            Run on any machine in the tailnet. No credentials needed, but it only lists the devices that
            machine can see, and reports only approved subnet routes.
          </p>
        )}

        <CommandBox command={command} />

        <div className="import-label-row">
          <label className="import-label" htmlFor="import-paste">
            {method === "api" ? "Paste the contents of tailnet.json" : "Paste the output"}
          </label>
          <label className="btn import-file">
            Choose file…
            <input type="file" accept=".json,application/json,text/plain" onChange={onFile} hidden />
          </label>
        </div>
        <textarea
          id="import-paste"
          ref={taRef}
          className="import-paste"
          value={text}
          onChange={(e) => setText(e.target.value)}
          spellCheck={false}
          placeholder={method === "api" ? '{"devices":[ … ]}' : '{"Self":{ … },"Peer":{ … }}'}
        />

        {parsed && "err" in parsed && (
          <p className="import-error" role="alert">
            {parsed.err}
          </p>
        )}
        {result && (
          <ImportPreview
            result={result}
            snapshot={snapshot}
            missingTags={tagPlan?.tags ?? []}
            tagsEditable={tagPlan?.editable ?? false}
            tagOwner={tagOwner}
            addTags={addTags}
            onAddTags={setAddTags}
          />
        )}

        <div className="consent-actions">
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="btn btn-primary" disabled={!result} onClick={apply}>
            Replace devices
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

const METHODS: { id: Method; label: string }[] = [
  { id: "api", label: "Tailscale API" },
  { id: "cli", label: "tailscale CLI" },
];

function MethodTabs({ method, onChange }: { method: Method; onChange: (m: Method) => void }) {
  return (
    <div className="seg import-seg" role="tablist">
      {METHODS.map((m) => (
        <button
          key={m.id}
          type="button"
          role="tab"
          aria-selected={method === m.id}
          className={method === m.id ? "active" : ""}
          onClick={() => onChange(m.id)}
        >
          {m.label}
        </button>
      ))}
    </div>
  );
}

function CommandBox({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable — the command is selectable */
    }
  };
  return (
    <div className="import-cmd">
      <pre>{command}</pre>
      <button type="button" className="btn" onClick={copy}>
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;
const PREVIEW_NAMES = 8;

/** What applying the import will change, shown before the user confirms. */
function ImportPreview({
  result,
  snapshot,
  missingTags,
  tagsEditable,
  tagOwner,
  addTags,
  onAddTags,
}: {
  result: ImportResult;
  snapshot: Snapshot;
  missingTags: string[];
  tagsEditable: boolean;
  tagOwner?: string;
  addTags: boolean;
  onAddTags: (v: boolean) => void;
}) {
  const newOwners = [...new Set([...result.owners, ...result.roles.map((r) => r.email)])].filter(
    (e) => !snapshot.users.some((u) => u.email === e),
  );
  const names = result.devices.slice(0, PREVIEW_NAMES).map((d) => d.name).join(", ");
  const more = result.devices.length - PREVIEW_NAMES;
  const have = new Set((snapshot.services ?? []).map((s) => serviceKey(s.name)));
  const newServices = result.services.filter((s) => !have.has(serviceKey(s.name))).length;
  const serviceNames = result.services.map((s) => s.name).join(", ");
  return (
    <div className="import-preview">
      <p>
        Found <strong>{plural(result.devices.length, "device")}</strong>: {names}
        {more > 0 ? `, +${more} more` : ""}
      </p>
      <ul>
        <li>
          Replaces the current <strong>{plural(snapshot.devices.length, "device")}</strong>.
        </li>
        {newOwners.length > 0 && (
          <li>Adds {plural(newOwners.length, "user")} (device owners and role holders not in the snapshot yet).</li>
        )}
        {result.roles.length > 0 && (
          <li>
            Sets {plural(result.roles.length, "role")}:{" "}
            {result.roles.slice(0, 5).map((r) => `${r.email} (${r.role})`).join(", ")}
            {result.roles.length > 5 ? `, +${result.roles.length - 5} more` : ""} (
            {result.source === "api" ? "from the users list" : "from this machine's capabilities"}).
          </li>
        )}
        {result.services.length > 0 && (
          <li>
            Merges {plural(result.services.length, "service")} by name ({serviceNames}
            ): {newServices} new, {result.services.length - newServices} updating the VIP of an existing one.
          </li>
        )}
        {(result.services.length > 0 || (snapshot.services?.length ?? 0) > 0) && (
          <li>Service backends are cleared (device ids change, and the paste doesn't say which devices back a service).</li>
        )}
        <li>Groups, hosts and external tailnets are left as they are.</li>
      </ul>
      {missingTags.length > 0 &&
        (tagsEditable ? (
          <label className="import-tags">
            <input type="checkbox" checked={addTags} onChange={(e) => onAddTags(e.target.checked)} />
            <span>
              Add <code>tagOwners</code> entries to the policy for {missingTags.map((t) => (
                <code key={t}>{t}</code>
              ))}
              {" "}
              (owner: {tagOwner ? <code>{tagOwner}</code> : "admins only"}). Without them the engine flags
              each tagged device as a problem.
            </span>
          </label>
        ) : (
          <p className="import-hint">
            The policy doesn't parse, so <code>tagOwners</code> for {plural(missingTags.length, "tag")} (
            {missingTags.join(", ")}) can't be added automatically.
          </p>
        ))}
    </div>
  );
}

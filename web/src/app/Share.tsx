import { useCallback, useEffect, useRef, useState } from "react";
import { canonicalView, useStore } from "../state/store";
import { createPortal } from "react-dom";
import { buildLongShareURL, buildShareURL } from "./share";
import { loadPendingShare } from "./useShareHydration";

export function ShareButton() {
  const [status, setStatus] = useState<"idle" | "copied" | "error">("idle");
  const [url, setUrl] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const policy = useStore((s) => s.policy);
  const snapshot = useStore((s) => s.snapshot);
  const probes = useStore((s) => s.probes);
  const sshUsers = useStore((s) => s.sshUsers);
  const view = useStore((s) => s.view);
  const leftTab = useStore((s) => s.leftTab);
  const activeProbe = useStore((s) => s.activeProbe);
  const grouping = useStore((s) => s.grouping);
  const activeLesson = useStore((s) => s.activeLesson);

  useEffect(() => {
    if (status === "idle") return;
    const t = setTimeout(() => setStatus("idle"), 2500);
    return () => clearTimeout(t);
  }, [status]);

  const state = () => ({
    policy,
    snapshot,
    probes,
    sshUsers,
    view: canonicalView({ leftTab, view }),
    activeProbe,
    grouping,
    activeLesson,
  });

  const finish = async (link: string) => {
    setUrl(link);
    try {
      await navigator.clipboard.writeText(link);
      setStatus("copied");
    } catch {
      setStatus("error");
    }
  };

  // Short link: stores a copy server-side (falls back to the long link if the API is down).
  const shareShort = async () => {
    setBusy(true);
    try {
      await finish(await buildShareURL(state()));
    } finally {
      setBusy(false);
      setOpen(false);
    }
  };

  // Long link: the scenario rides in the URL; nothing is sent to the server.
  const shareLong = async () => {
    setOpen(false);
    await finish(buildLongShareURL(state()));
  };

  return (
    <span className="share">
      <button className="btn" onClick={() => setOpen(true)} title="Copy a link to this exact scenario">
        {status === "copied" ? "link copied ✓" : status === "error" ? "select & copy ↓" : "share"}
      </button>
      {status === "error" && url && (
        <input
          className="share-fallback"
          readOnly
          value={url}
          onFocus={(e) => e.currentTarget.select()}
        />
      )}
      {open && <ShareNotice busy={busy} onShort={shareShort} onLong={shareLong} onCancel={() => setOpen(false)} />}
    </span>
  );
}

/**
 * Shown before a share link is made. A short link copies the whole scenario to
 * the server, so say exactly what that means before anything leaves the browser.
 * The retention figure mirrors shareTTL in server/main.go.
 */
function ShareNotice({
  busy,
  onShort,
  onLong,
  onCancel,
}: {
  busy: boolean;
  onShort: () => void;
  onLong: () => void;
  onCancel: () => void;
}) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    cancelRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !busy && onCancel();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [busy, onCancel]);

  return createPortal(
    <div className="consent-backdrop" role="presentation">
      <div className="consent-card" role="dialog" aria-modal="true" aria-labelledby="share-title">
        <h2 id="share-title">Create a share link?</h2>
        <p>
          A short link saves a <strong>copy of this scenario on the Tailground server</strong> so
          anyone with the link can open it. Check it for anything you wouldn't want others to read.
        </p>
        <ul>
          <li>
            <strong>What's stored:</strong> your whole policy <em>including comments</em>, and the
            tailnet — user emails, device names, tags, IP addresses and routes — plus your probes.
          </li>
          <li>
            <strong>For how long:</strong> 1 year after the link was last opened (each open renews
            it). It can't be deleted sooner.
          </li>
          <li>
            <strong>Who can read it:</strong> anyone who has the link. It is not encrypted or
            password-protected.
          </li>
        </ul>
        <p className="consent-hint">
          Remove sensitive names, emails and comments first. Or use a long link: nothing is stored
          on the server — the scenario is inside the URL itself (longer, and some apps truncate it).
        </p>
        <div className="consent-actions">
          <button ref={cancelRef} className="btn" onClick={onCancel} disabled={busy}>
            Cancel
          </button>
          <button className="btn" onClick={onLong} disabled={busy}>
            Copy long link
          </button>
          <button className="btn btn-primary" onClick={onShort} disabled={busy}>
            {busy ? "Creating…" : "Create short link"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export function ShareBanner() {
  const loaded = useStore((s) => s.loadedFromShare);
  const failed = useStore((s) => s.shareFailed);
  const dismiss = useStore((s) => s.dismissShareBanner);
  if (failed)
    return (
      <div className="share-banner">
        <span>Couldn't load that shared link — it may have expired, been truncated, or the server is unreachable. Showing the default scenario.</span>
        <button className="btn" onClick={dismiss}>
          dismiss
        </button>
      </div>
    );
  if (!loaded) return null;
  return (
    <div className="share-banner">
      <span>
        Loaded from a shared link. Your edits stay local — they aren't saved
        back to the link.
      </span>
      <button className="btn" onClick={dismiss}>
        dismiss
      </button>
    </div>
  );
}

/**
 * Notice shown before a shared link is loaded. The scenario is authored by
 * someone else and is not verified, so nothing from it is fetched or rendered
 * until the visitor says yes; "no" returns to the default landing page.
 */
export function ShareConsent() {
  const pending = useStore((s) => s.pendingShare);
  const setPending = useStore((s) => s.setPendingShare);
  const declineRef = useRef<HTMLButtonElement>(null);
  const startFresh = useStore((s) => s.startFresh);

  const decline = useCallback(() => {
    // Load the built-in default scenario into the playground and point the URL at it.
    startFresh();
    window.history.replaceState(null, "", window.location.pathname + "#/");
  }, [startFresh]);

  const accept = () => {
    if (!pending) return;
    setPending(null);
    void loadPendingShare(pending);
  };

  useEffect(() => {
    if (!pending) return;
    declineRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && decline();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [pending, decline]);

  if (!pending) return null;
  return (
    <div className="consent-backdrop" role="presentation">
      <div className="consent-card" role="dialog" aria-modal="true" aria-labelledby="consent-title">
        <h2 id="consent-title">Open a shared scenario?</h2>
        <p>
          This link contains a policy and tailnet that <strong>someone else created</strong>.
          Tailground can't verify who made it or whether it's accurate, so don't treat it as
          official guidance or enter anything sensitive.
        </p>
        <ul>
          <li>It runs only in your browser; your edits are never saved back to the link.</li>
          <li>
            {pending.kind === "id"
              ? "It will be fetched from this site's server."
              : "The scenario is carried inside the link itself."}
          </li>
        </ul>
        <div className="consent-actions">
          <button ref={declineRef} className="btn" onClick={decline}>
            No, start fresh
          </button>
          <button className="btn btn-primary" onClick={accept}>
            Load scenario
          </button>
        </div>
      </div>
    </div>
  );
}

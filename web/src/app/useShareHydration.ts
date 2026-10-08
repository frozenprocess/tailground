import { useEffect, useRef } from "react";
import { useStore } from "../state/store";
import {
  decodeShare,
  fetchShareToken,
  isOwnShare,
  shareIdFromHash,
  shareRouteFromHash,
  shareTokenFromHash,
  type PendingShare,
} from "./share";
import { parseHash, toHash } from "./useHashRoute";

/** Fetch/decode a share link and load it into the store. Call only once the visitor has agreed. */
export async function loadPendingShare(p: PendingShare) {
  const { hydrateFromShare, failShare } = useStore.getState();
  const tok = p.kind === "id" ? await fetchShareToken(p.id) : p.token;
  let state = tok ? decodeShare(tok) : null;
  if (!state) {
    failShare();
    return;
  }
  if (p.kind === "token" && p.route) {
    const { view, lesson } = parseHash(p.route);
    state = { ...state, view, activeLesson: lesson ?? state.activeLesson };
  }
  hydrateFromShare(state);
  // The URL was stripped before the scenario loaded; point it at the view the
  // scenario opens on (the store→hash mirror may not have run).
  window.history.replaceState(
    null,
    "",
    window.location.pathname + toHash(state.view ?? "matrix", state.activeLesson ?? null),
  );
}

/**
 * Whenever the URL fragment carries a short link (`#/s/<id>`, fetched from the
 * API) or a share token (`#s=<token>`, or one glued onto a route like
 * `#/graph/#s=<token>`, whose route then wins over the token's saved view) —
 * on first load, or later when a link is pasted into an already-open tab (a
 * fragment-only navigation fires `hashchange`, not a reload) — strip it from
 * the address bar (so a refresh doesn't re-prompt and the normal hash router
 * takes over), then hold it as `pendingShare`. Nothing from the link is
 * fetched, decoded or rendered until the visitor accepts the notice
 * (ShareConsent) — except links this browser created itself. Runs before
 * useHashRoute, and its listener is registered first, so the router only ever
 * sees the stripped hash.
 */
export function useShareHydration() {
  const setPending = useStore((s) => s.setPendingShare);
  const initial = useRef(false);

  useEffect(() => {
    const load = () => {
      const hash = window.location.hash;
      const id = shareIdFromHash(hash);
      const token = id ? null : shareTokenFromHash(hash);
      let pending: PendingShare;
      let keep: string | null = null;
      if (id) {
        pending = { kind: "id", id };
      } else if (token) {
        keep = shareRouteFromHash(hash);
        pending = { kind: "token", token, route: keep };
      } else {
        return;
      }

      // Strip immediately so useHashRoute sees a normal hash this tick.
      window.history.replaceState(null, "", window.location.pathname + (keep ?? "#/"));

      if (id && isOwnShare(id)) void loadPendingShare(pending);
      else setPending(pending);
    };

    // StrictMode re-runs this effect; the stripped hash makes a second load a
    // no-op anyway, but the ref keeps it explicit.
    if (!initial.current) {
      initial.current = true;
      load();
    }
    window.addEventListener("hashchange", load);
    return () => window.removeEventListener("hashchange", load);
  }, [setPending]);
}

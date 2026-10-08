import { encodeShare, type SharedState } from "./shareCodec";

const API = "/api/share";

// Short links this browser created: opening your own link needs no warning.
const OWN_KEY = "tailground.ownShares";
const OWN_MAX = 200;

function readOwn(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(OWN_KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string") : [];
  } catch {
    return [];
  }
}

function rememberOwnShare(id: string) {
  try {
    localStorage.setItem(OWN_KEY, JSON.stringify([id, ...readOwn().filter((x) => x !== id)].slice(0, OWN_MAX)));
  } catch {
    // storage unavailable: the visitor just sees the notice on their own links
  }
}

export function isOwnShare(id: string): boolean {
  return readOwn().includes(id);
}

/** The long, fully self-contained link: the whole scenario rides in the fragment. */
export function buildLongShareURL(state: SharedState): string {
  return `${window.location.origin}${window.location.pathname}#s=${encodeShare(state)}`;
}

/**
 * Shareable URL for the current origin + base path. Prefers a short link
 * (`#/s/<id>`, the scenario stored server-side); if the API is unreachable or
 * refuses, falls back to the long fragment link so sharing never fails.
 */
export async function buildShareURL(state: SharedState): Promise<string> {
  const token = encodeShare(state);
  try {
    const res = await fetch(API, {
      method: "POST",
      body: token,
      headers: { "Content-Type": "text/plain" },
      signal: AbortSignal.timeout(5000),
    });
    if (res.ok) {
      const { id } = (await res.json()) as { id?: string };
      if (id && /^[A-Za-z0-9]{8,16}$/.test(id)) {
        rememberOwnShare(id);
        return `${window.location.origin}${window.location.pathname}#/s/${id}`;
      }
    }
  } catch {
    // fall through to the long link
  }
  return `${window.location.origin}${window.location.pathname}#s=${token}`;
}

/** Fetch a stored share's token by short-link ID; null if unknown or unreachable. */
export async function fetchShareToken(id: string): Promise<string | null> {
  try {
    const res = await fetch(`${API}/${id}`, { signal: AbortSignal.timeout(10000) });
    return res.ok ? (await res.text()).trim() : null;
  } catch {
    return null;
  }
}


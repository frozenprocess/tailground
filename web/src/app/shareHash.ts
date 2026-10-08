/** The short-link ID in a URL fragment (`#/s/<id>`), if any. */
export function shareIdFromHash(hash: string): string | null {
  return hash.match(/^#\/s\/([A-Za-z0-9]{8,16})\/?$/)?.[1] ?? null;
}

/**
 * The share token in a URL fragment, if any. Canonical links are `#s=<token>`,
 * but a token glued onto a route (`#/graph/#s=<token>`) still counts — the
 * browser folds everything after the first `#` into one fragment.
 */
export function shareTokenFromHash(hash: string): string | null {
  return hash.match(/(?:^|#)s=([A-Za-z0-9_-]+)/)?.[1] ?? null;
}

/** The route a share token was glued onto (`#/graph/#s=…` → `#/graph`), or null. */
export function shareRouteFromHash(hash: string): string | null {
  const i = hash.search(/(?:^|#)s=[A-Za-z0-9_-]/);
  const route = i > 0 ? hash.slice(0, i).replace(/[/#]+$/, "") : "";
  return route.startsWith("#/") ? route : null;
}

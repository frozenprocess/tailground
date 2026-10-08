// Public sharing facade: stable compressed fragments plus optional server short links.
export { decodeShare, type SharedState } from "./shareCodec";
export { buildLongShareURL, buildShareURL, fetchShareToken, isOwnShare } from "./shareTransport";
export { shareIdFromHash, shareTokenFromHash, shareRouteFromHash } from "./shareHash";

/** A share link held until the visitor accepts the notice. */
export type PendingShare =
  | { kind: "id"; id: string }
  | { kind: "token"; token: string; route: string | null };

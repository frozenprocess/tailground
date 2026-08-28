// The engine result's composite keys (mirrors engine/eval/keys.go).

type End = number | string;

/** The "src->dst" key of `matrices[probe]`, `grants` and `posture.denials`.
 *  Each end is an actor id or a matrix column ("route:…", "svc:…", "ext:…"). */
export const pairKey = (src: End, dst: End): string => `${src}->${dst}`;

/** The [src, dst] ends of a pairKey (or of an sshKey's pair), as written. */
export function pairEnds(key: string): [string, string] {
  const [src, dst] = key.split("->");
  return [src, dst];
}

/** The "user@srcID->dstID" key of `ssh`. */
export const sshKey = (user: string, src: End, dst: End): string => `${user}@${pairKey(src, dst)}`;

/** The [user, pairKey] halves of an sshKey. */
export function splitSSHKey(key: string): [string, string] {
  const [user, pair] = key.split("@");
  return [user, pair];
}

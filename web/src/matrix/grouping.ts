import type { Actor } from "../engine/types";
import type { Grouping } from "../state/store";

export interface ActorGroup {
  key: string;
  label: string;
  actors: Actor[];
}

/**
 * Order actors into labelled groups for the matrix axes. "flat" is a single
 * unnamed group; "user" groups user-owned devices by owner (tagged devices fall
 * into "tagged"); "tag" groups by first tag (user devices fall into "untagged").
 */
export function groupActors(actors: Actor[], mode: Grouping): ActorGroup[] {
  if (mode === "flat") {
    return [{ key: "all", label: "", actors: sortByName(actors) }];
  }

  const buckets = new Map<string, Actor[]>();
  const push = (k: string, a: Actor) => {
    const arr = buckets.get(k) ?? [];
    arr.push(a);
    buckets.set(k, arr);
  };

  for (const a of actors) {
    if (mode === "user") {
      push(a.kind === "user" && a.user ? a.user : "tagged", a);
    } else {
      const tag = a.tags?.[0];
      push(tag ?? "untagged", a);
    }
  }

  const keys = [...buckets.keys()].sort((x, y) => {
    // Keep the catch-all bucket last.
    const catchAll = mode === "user" ? "tagged" : "untagged";
    if (x === catchAll) return 1;
    if (y === catchAll) return -1;
    return x.localeCompare(y);
  });

  return keys.map((key) => ({
    key,
    label: key,
    actors: sortByName(buckets.get(key)!),
  }));
}

function sortByName(actors: Actor[]): Actor[] {
  return [...actors].sort((a, b) => a.name.localeCompare(b.name));
}

/** Flattened actor order matching the grouped layout, for column headers. */
export function flatten(groups: ActorGroup[]): Actor[] {
  return groups.flatMap((g) => g.actors);
}

import { useStore } from "../state/store";
import type { Snapshot } from "../engine/types";

type ListKey = "devices" | "users" | "services" | "externalTailnets";
type ListItem<K extends ListKey> = NonNullable<Snapshot[K]>[number];

export interface SnapshotList<K extends ListKey> {
  items: ListItem<K>[];
  set: (i: number, next: ListItem<K>) => void;
  remove: (i: number) => void;
  add: (item: ListItem<K>) => void;
}

/** Row edits on one of the snapshot's lists (a missing optional list is empty). */
export function useSnapshotList<K extends ListKey>(key: K): SnapshotList<K> {
  const snapshot = useStore((s) => s.snapshot);
  const setSnapshot = useStore((s) => s.setSnapshot);
  const items = (snapshot[key] ?? []) as ListItem<K>[];
  const put = (next: ListItem<K>[]) => setSnapshot({ ...snapshot, [key]: next });
  return {
    items,
    set: (i, next) => put(items.map((x, j) => (j === i ? next : x))),
    remove: (i) => put(items.filter((_, j) => j !== i)),
    add: (item) => put([...items, item]),
  };
}

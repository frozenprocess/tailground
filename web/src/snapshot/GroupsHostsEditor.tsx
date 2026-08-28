import { useEffect, useRef, useState } from "react";
import type { Snapshot } from "../engine/types";
import { Panel } from "../ui/Panel";
import { splitList } from "./splitList";

export type CollapseFactory = (id: string, dflt: boolean) => {
  collapsible: true;
  id: string;
  collapsed: boolean;
  onCollapsedChange: (c: boolean) => void;
};

// Groups (name → members) and hosts (name → CIDR/IP) as small table editors.
// The policy's own `groups` / `hosts` blocks are authoritative; these seed the
// snapshot so `group:` / host references in rules resolve during evaluation.
// Local row state (rather than deriving straight off the record) keeps a key
// editable while it is momentarily blank.
export function GroupsHostsEditor({
  snapshot,
  onChange,
  collapse,
}: {
  snapshot: Snapshot;
  onChange: (patch: Partial<Snapshot>) => void;
  collapse: CollapseFactory;
}) {
  type Row = { k: string; v: string };
  const toGroupRows = (s: Snapshot): Row[] =>
    Object.entries(s.groups ?? {}).map(([k, v]) => ({ k, v: v.join(", ") }));
  const toHostRows = (s: Snapshot): Row[] =>
    Object.entries(s.hosts ?? {}).map(([k, v]) => ({ k, v }));

  const [groupRows, setGroupRows] = useState<Row[]>(() => toGroupRows(snapshot));
  const [hostRows, setHostRows] = useState<Row[]>(() => toHostRows(snapshot));
  const lastGroups = useRef(JSON.stringify(snapshot.groups ?? {}));
  const lastHosts = useRef(JSON.stringify(snapshot.hosts ?? {}));

  // Re-sync when groups/hosts change from outside (e.g. a preset load), but not
  // when the change is the echo of what we just typed.
  useEffect(() => {
    const incoming = JSON.stringify(snapshot.groups ?? {});
    if (incoming !== lastGroups.current) {
      setGroupRows(toGroupRows(snapshot));
      lastGroups.current = incoming;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [snapshot.groups]);
  useEffect(() => {
    const incoming = JSON.stringify(snapshot.hosts ?? {});
    if (incoming !== lastHosts.current) {
      setHostRows(toHostRows(snapshot));
      lastHosts.current = incoming;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [snapshot.hosts]);

  const emitGroups = (rows: Row[]) => {
    setGroupRows(rows);
    const next: Record<string, string[]> = {};
    for (const { k, v } of rows) {
      const key = k.trim();
      if (key) next[key] = splitList(v) ?? [];
    }
    lastGroups.current = JSON.stringify(next);
    onChange({ groups: Object.keys(next).length ? next : undefined });
  };
  const emitHosts = (rows: Row[]) => {
    setHostRows(rows);
    const next: Record<string, string> = {};
    for (const { k, v } of rows) {
      const key = k.trim();
      if (key) next[key] = v.trim();
    }
    lastHosts.current = JSON.stringify(next);
    onChange({ hosts: Object.keys(next).length ? next : undefined });
  };

  const editRow = (rows: Row[], i: number, patch: Partial<Row>) =>
    rows.map((r, j) => (j === i ? { ...r, ...patch } : r));

  return (
    <>
      <Panel {...collapse("snap-groups", true)} title="Groups" badge={groupRows.length || undefined}>
        <div className="snap-sec">
        <table>
          <thead>
            <tr>
              <th>name</th>
              <th>members</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {groupRows.map((row, i) => (
              <tr key={i}>
                <td className="tags-cell">
                  <input
                    type="text"
                    placeholder="group:eng"
                    value={row.k}
                    onChange={(e) => emitGroups(editRow(groupRows, i, { k: e.target.value }))}
                  />
                </td>
                <td>
                  <input
                    type="text"
                    placeholder="alice@example.com, group:ops"
                    value={row.v}
                    onChange={(e) => emitGroups(editRow(groupRows, i, { v: e.target.value }))}
                  />
                </td>
                <td>
                  <button
                    className="row-del"
                    onClick={() => emitGroups(groupRows.filter((_, j) => j !== i))}
                  >
                    ✕
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <button
          className="add-row"
          onClick={() => setGroupRows([...groupRows, { k: "", v: "" }])}
        >
          + group
        </button>
        </div>
      </Panel>

      <Panel {...collapse("snap-hosts", true)} title="Hosts" badge={hostRows.length || undefined}>
        <div className="snap-sec">
        <table>
          <thead>
            <tr>
              <th>name</th>
              <th>CIDR / IP</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {hostRows.map((row, i) => (
              <tr key={i}>
                <td className="tags-cell">
                  <input
                    type="text"
                    placeholder="prod-net"
                    value={row.k}
                    onChange={(e) => emitHosts(editRow(hostRows, i, { k: e.target.value }))}
                  />
                </td>
                <td className="tags-cell">
                  <input
                    type="text"
                    placeholder="10.0.0.0/24"
                    value={row.v}
                    onChange={(e) => emitHosts(editRow(hostRows, i, { v: e.target.value }))}
                  />
                </td>
                <td>
                  <button
                    className="row-del"
                    onClick={() => emitHosts(hostRows.filter((_, j) => j !== i))}
                  >
                    ✕
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <button
          className="add-row"
          onClick={() => setHostRows([...hostRows, { k: "", v: "" }])}
        >
          + host
        </button>
        <p style={{ color: "var(--text-subtle)", fontSize: 11, marginTop: 4 }}>
          The policy's own <code>groups</code> / <code>hosts</code> blocks are
          authoritative; these seed the snapshot for reference.
        </p>
        </div>
      </Panel>
    </>
  );
}

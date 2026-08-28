import { useMemo, useState } from "react";
import { useStore, type Grouping } from "../state/store";
import type { Actor, EngineResult, SSHVerdict } from "../engine/types";
import { pairKey, splitSSHKey, sshKey } from "../engine/keys";
import { flatten, groupActors } from "./grouping";
import { ProbeEditor } from "./ProbeEditor";
import { Panel } from "../ui/Panel";
import { FatalNotice, UnhomedNotice } from "../impact/EngineNotice";
import { isFatal } from "../engine/diagnostics";
import { usePaneMax } from "../app/usePaneMax";
import { ImpactTabs } from "../impact/ImpactTabs";
import { AnalysisStrip } from "../impact/AnalysisStrip";

const GROUPINGS: { id: Grouping; label: string }[] = [
  { id: "user", label: "By owner" },
  { id: "tag", label: "By tag" },
  { id: "flat", label: "Flat" },
];

const SSH_PREFIX = "ssh:";

export function MatrixView() {
  const result = useStore((s) => s.result);
  const evaluating = useStore((s) => s.evaluating);
  const activeProbe = useStore((s) => s.activeProbe);
  const setActiveProbe = useStore((s) => s.setActiveProbe);
  const grouping = useStore((s) => s.grouping);
  const setGrouping = useStore((s) => s.setGrouping);
  const selectedPair = useStore((s) => s.selectedPair);
  const selectPair = useStore((s) => s.selectPair);
  const focusedActor = useStore((s) => s.focusedActor);
  const setFocusedActor = useStore((s) => s.setFocusedActor);
  const sshUsers = useStore((s) => s.sshUsers);
  const setSSHUsers = useStore((s) => s.setSSHUsers);

  const probes = useStore((s) => s.probes);
  const probeNames = result
    ? probes.map((p) => p.name).filter((n) => n in result.matrices)
    : [];
  const hasSSH = !!result?.ssh && Object.keys(result.ssh).length > 0;
  const sshMode = activeProbe.startsWith(SSH_PREFIX);
  const sshUser = sshMode ? activeProbe.slice(SSH_PREFIX.length) : "";

  const groups = useMemo(
    () => (result ? groupActors(result.actors, grouping) : []),
    [result, grouping],
  );
  const cols = useMemo(() => flatten(groups), [groups]);

  const fatal = result?.diagnostics?.some(isFatal);

  // Open a device in the inspector from a Route/Service panel row.
  const focusDevice = (id: number) => {
    selectPair(null);
    setFocusedActor(id);
  };

  const [newUser, setNewUser] = useState("");

  return (
    <Panel
      id="impact"
      title="Reachability"
      tabs={<ImpactTabs />}
      badge={evaluating ? "evaluating…" : undefined}
      {...usePaneMax("impact")}
    >
      <div className="matrix-toolbar">
        <div className="seg">
          {probeNames.map((p) => (
            <button
              key={p}
              className={p === activeProbe ? "active" : ""}
              onClick={() => setActiveProbe(p)}
            >
              {p}
            </button>
          ))}
          {hasSSH &&
            sshUsers.map((u) => (
              <button
                key={u}
                className={activeProbe === SSH_PREFIX + u ? "active" : ""}
                onClick={() => setActiveProbe(SSH_PREFIX + u)}
                title={`SSH as ${u}`}
              >
                ssh:{u}
              </button>
            ))}
        </div>
        {hasSSH && (
          <input
            className="ssh-user-add"
            placeholder="+ ssh user"
            value={newUser}
            onChange={(e) => setNewUser(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && newUser.trim()) {
                const u = newUser.trim();
                if (!sshUsers.includes(u)) setSSHUsers([...sshUsers, u]);
                setNewUser("");
              }
            }}
          />
        )}
        <span className="spacer" style={{ flex: 1 }} />
        <ProbeEditor />
        <div className="seg">
          {GROUPINGS.map((g) => (
            <button
              key={g.id}
              className={g.id === grouping ? "active" : ""}
              onClick={() => setGrouping(g.id)}
            >
              {g.label}
            </button>
          ))}
        </div>
      </div>

      <div className="pane-body">
        {!fatal && <UnhomedNotice />}
        {!result ? (
          <p className="empty">Waiting for the engine…</p>
        ) : fatal ? (
          <FatalNotice what="matrix" />
        ) : cols.length === 0 ? (
          <p className="empty">Add some devices to the snapshot to see a matrix.</p>
        ) : (
          <div className="matrix-scroll">
            <MatrixTable
              result={result}
              probe={activeProbe}
              sshUser={sshUser}
              groups={groups}
              cols={cols}
              selected={selectedPair}
              focused={focusedActor}
              onFocus={(id) => setFocusedActor(focusedActor === id ? null : id)}
              onSelect={(src, dst) =>
                selectPair(
                  selectedPair?.src === src && selectedPair?.dst === dst
                    ? null
                    : { src, dst },
                )
              }
            />
            {sshMode && (
              <p className="matrix-legend-ssh">
                <span className="v-accept">accept</span> connects directly ·{" "}
                <span className="v-check">check</span> needs browser re-auth ·{" "}
                <span className="v-reject">reject</span> / <span>deny</span> blocked
              </p>
            )}
          </div>
        )}
      </div>

      {result && (
        <AnalysisStrip
          result={result}
          probe={activeProbe}
          sshMode={sshMode}
          selectedPair={selectedPair}
          onClosePair={() => selectPair(null)}
          focusedActor={focusedActor}
          onCloseDevice={() => setFocusedActor(null)}
          onFocusDevice={focusDevice}
        />
      )}
    </Panel>
  );
}

interface TableProps {
  result: EngineResult;
  probe: string;
  sshUser: string;
  groups: ReturnType<typeof groupActors>;
  cols: Actor[];
  selected: { src: number; dst: number } | null;
  focused: number | null;
  onFocus: (id: number) => void;
  onSelect: (src: number, dst: number) => void;
}

function MatrixTable({
  result,
  probe,
  sshUser,
  groups,
  cols,
  selected,
  focused,
  onFocus,
  onSelect,
}: TableProps) {
  const ssh = !!sshUser;
  const cells = ssh ? {} : (result.matrices[probe] ?? {});
  const badgeUsers = sshUsersIn(result);

  return (
    <table className="matrix">
      <thead>
        <tr>
          <th className="rowhead corner">src → dst</th>
          {cols.map((c) => (
            <th
              key={c.id}
              className={`clickable${c.id === focused ? " focused" : ""}`}
              title={actorTitle(c)}
              onClick={() => onFocus(c.id)}
            >
              <ActorLabel actor={c} />
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {groups.map((g) => (
          <GroupRows
            key={g.key}
            group={g}
            cols={cols}
            cells={cells}
            sshUser={sshUser}
            result={result}
            badgeUsers={badgeUsers}
            selected={selected}
            focused={focused}
            onFocus={onFocus}
            onSelect={onSelect}
            showGroupHeader={groups.length > 1}
          />
        ))}
      </tbody>
    </table>
  );
}

function GroupRows({
  group,
  cols,
  cells,
  sshUser,
  result,
  badgeUsers,
  selected,
  focused,
  onFocus,
  onSelect,
  showGroupHeader,
}: {
  group: { key: string; label: string; actors: Actor[] };
  cols: Actor[];
  cells: Record<string, string>;
  sshUser: string;
  result: EngineResult;
  badgeUsers: string[];
  selected: { src: number; dst: number } | null;
  focused: number | null;
  onFocus: (id: number) => void;
  onSelect: (src: number, dst: number) => void;
  showGroupHeader: boolean;
}) {
  const sshMode = !!sshUser;
  return (
    <>
      {showGroupHeader && (
        <tr>
          <th className="group" colSpan={cols.length + 1}>
            {group.label}
          </th>
        </tr>
      )}
      {group.actors.map((src) => (
        <tr key={src.id}>
          <th
            className={`rowhead clickable${src.id === focused ? " focused" : ""}`}
            title={actorTitle(src)}
            onClick={() => onFocus(src.id)}
          >
            <ActorLabel actor={src} />
          </th>
          {cols.map((dst) => {
            if (src.id === dst.id) return <td key={dst.id} className="cell self" />;
            const key = pairKey(src.id, dst.id);
            const isSel = selected?.src === src.id && selected?.dst === dst.id;
            const inFocus =
              focused != null && (src.id === focused || dst.id === focused);

            if (sshMode) {
              const v = result.ssh?.[`${sshUser}@${key}`] as SSHVerdict | undefined;
              const cls = v ?? "none";
              return (
                <td
                  key={dst.id}
                  className={`cell ssh-cell v-${cls}${isSel ? " sel" : ""}${inFocus ? " focusrow" : ""}`}
                  title={`${src.name} → ${dst.name} · SSH as ${sshUser}: ${v ?? "no rule"}`}
                  onClick={() => onSelect(src.id, dst.id)}
                >
                  {SSH_GLYPH[cls] ?? "·"}
                </td>
              );
            }

            const verdict = cells[key] === "allow" ? "allow" : "deny";
            const badge = sshBadge(result, badgeUsers, src.id, dst.id);
            const posture = result.posture?.denials?.[key];
            return (
              <td
                key={dst.id}
                className={`cell ${verdict}${isSel ? " sel" : ""}${inFocus ? " focusrow" : ""}${posture ? " posture" : ""}`}
                title={
                  posture
                    ? `${cellTitle(result, src, dst)}\n  posture: ${posture}`
                    : cellTitle(result, src, dst)
                }
                onClick={() => onSelect(src.id, dst.id)}
              >
                {verdict === "allow" ? "●" : "·"}
                {badge && <span className="ssh-dot"> {badge}</span>}
                {posture && <span className="ssh-dot"> P</span>}
              </td>
            );
          })}
        </tr>
      ))}
    </>
  );
}

const SSH_GLYPH: Record<string, string> = {
  accept: "●",
  check: "◐",
  reject: "✕",
  deny: "·",
  none: "·",
};

function ActorLabel({ actor }: { actor: Actor }) {
  const sub = actor.kind === "user" ? actor.user : actor.tags?.join(", ");
  return (
    <span className="actor-label">
      <span>{actor.name}</span>
      {sub && <span className="sub">{sub}</span>}
    </span>
  );
}

function actorTitle(a: Actor): string {
  const parts = [a.name];
  if (a.user) parts.push(`user ${a.user}`);
  if (a.tags?.length) parts.push(a.tags.join(" "));
  if (a.ips.length) parts.push(a.ips.join(" "));
  return parts.join(" · ");
}

function cellTitle(result: EngineResult, src: Actor, dst: Actor): string {
  const lines: string[] = [`${src.name} → ${dst.name}`];
  for (const [probe, m] of Object.entries(result.matrices)) {
    lines.push(`  ${probe}: ${m[pairKey(src.id, dst.id)] === "allow" ? "allow" : "deny"}`);
  }
  const grants = result.grants?.[pairKey(src.id, dst.id)];
  if (grants?.length) lines.push(`  grants: ${grants.join(", ")}`);
  return lines.join("\n");
}

function sshUsersIn(result: EngineResult): string[] {
  const set = new Set<string>();
  for (const k of Object.keys(result.ssh ?? {})) set.add(splitSSHKey(k)[0]);
  return [...set];
}

function sshBadge(
  result: EngineResult,
  users: string[],
  src: number,
  dst: number,
): string | null {
  if (!result.ssh) return null;
  let best: string | null = null;
  for (const u of users) {
    const v = result.ssh[sshKey(u, src, dst)];
    if (v === "accept") best = "S";
    else if (v === "check" && best !== "S") best = "s";
  }
  return best;
}

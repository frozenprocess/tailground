import { useStore } from "../state/store";
import { Panel } from "../ui/Panel";
import { DiagMark } from "../ui/DiagMark";
import { forTarget } from "../engine/diagnostics";
import type { Diagnostic } from "../engine/types";
import type { CollapseFactory } from "./GroupsHostsEditor";
import { DeviceRow } from "./DeviceRow";
import { ExternalTailnetCard } from "./ExternalTailnetCard";
import { ServiceRow } from "./ServiceRow";
import { SnapTable } from "./SnapTable";
import { UserRow } from "./UserRow";
import { useSnapshotList } from "./useSnapshotList";

const NO_DIAGS: Diagnostic[] = [];

// Snapshot problems are shown here, on the item they're about (device rows,
// section headers) — not under the policy editor.
function useDiagnostics() {
  return useStore((s) => s.result?.diagnostics ?? NO_DIAGS);
}

const nextId = (items: { id: number }[]) => items.reduce((m, x) => Math.max(m, x.id), 0) + 1;

/** A section header badge: the item count plus the section's diagnostics. */
function countBadge(count: number, diags: Diagnostic[]) {
  if (!count && !diags.length) return undefined;
  return (
    <>
      {count || null}
      <DiagMark diags={diags} />
    </>
  );
}

export function DevicesSection({ collapse }: { collapse: CollapseFactory }) {
  const devices = useSnapshotList("devices");
  const users = useSnapshotList("users");
  const diagnostics = useDiagnostics();
  const devicesCollapse = collapse("snap-devices", false);
  const deviceDiags = diagnostics.filter((d) => d.target?.startsWith("device:"));
  const emails = users.items.map((u) => u.email);
  const id = nextId(devices.items);

  return (
    <Panel
      {...devicesCollapse}
      title="Devices"
      badge={
        <>
          {devices.items.length}
          {/* Rows carry their own marks; the header stands in only while
              they're hidden, so a message is never visible twice. */}
          {devicesCollapse.collapsed && <DiagMark diags={deviceDiags} />}
        </>
      }
    >
      <SnapTable
        heads={[
          "name",
          "user",
          "tags",
          ["ip", "ipv4 / ipv6 — blank auto-assigns from the CGNAT / ULA range"],
          "os",
          "routes",
          "posture",
        ]}
        addLabel="+ device"
        onAdd={() => devices.add({ id, name: `device-${id}` })}
      >
        {devices.items.map((d, i) => (
          <DeviceRow
            key={d.id}
            device={d}
            diags={forTarget(diagnostics, `device:${d.name}`)}
            users={emails}
            onChange={(next) => devices.set(i, next)}
            onDelete={() => devices.remove(i)}
          />
        ))}
      </SnapTable>
    </Panel>
  );
}

export function UsersSection({ collapse }: { collapse: CollapseFactory }) {
  const users = useSnapshotList("users");
  const id = nextId(users.items);
  return (
    <Panel {...collapse("snap-users", true)} title="Users" badge={users.items.length}>
      <SnapTable
        heads={[
          "email",
          "role",
          ["shared", "joined via a share invitation — matched by autogroup:shared"],
        ]}
        addLabel="+ user"
        onAdd={() => users.add({ id, email: `user${id}@example.com` })}
      >
        {users.items.map((u, i) => (
          <UserRow
            key={u.id}
            user={u}
            onChange={(patch) => users.set(i, { ...u, ...patch })}
            onDelete={() => users.remove(i)}
          />
        ))}
      </SnapTable>
    </Panel>
  );
}

export function ServicesSection({ collapse }: { collapse: CollapseFactory }) {
  const services = useSnapshotList("services");
  const devices = useSnapshotList("devices");
  const diags = forTarget(useDiagnostics(), "services");
  return (
    <Panel
      {...collapse("snap-services", true)}
      title="Tailscale Services"
      badge={countBadge(services.items.length, diags)}
    >
      <SnapTable
        heads={["name", "VIP", "backends"]}
        addLabel="+ service"
        onAdd={() => services.add({ name: `svc:service-${services.items.length + 1}`, vip: "" })}
      >
        {services.items.map((s, i) => (
          <ServiceRow
            key={i}
            service={s}
            devices={devices.items}
            onChange={(next) => services.set(i, next)}
            onDelete={() => services.remove(i)}
          />
        ))}
      </SnapTable>
    </Panel>
  );
}

export function ExternalSection({ collapse }: { collapse: CollapseFactory }) {
  const externalTailnets = useSnapshotList("externalTailnets");
  const externalShare = useStore((s) => s.result?.externalShare);
  const diags = forTarget(useDiagnostics(), "external");
  return (
    <Panel
      {...collapse("snap-external", true)}
      title="External tailnets"
      badge={countBadge(externalTailnets.items.length, diags)}
    >
      <div className="snap-sec">
        {externalTailnets.items.map((et, i) => (
          <ExternalTailnetCard
            key={i}
            tailnet={et}
            config={(externalShare?.tailnets ?? []).find((t) => t.alias === et.alias)}
            onChange={(next) => externalTailnets.set(i, next)}
            onDelete={() => externalTailnets.remove(i)}
          />
        ))}
        <button
          className="add-row"
          onClick={() =>
            externalTailnets.add({
              alias: `partner-${externalTailnets.items.length + 1}`,
              nodes: [],
            })
          }
        >
          + external tailnet
        </button>
        <p style={{ color: "var(--text-subtle)", fontSize: 11, marginTop: 4 }}>
          The policy's <code>externalTailnets</code> section configures the relationship
          (external ID, <code>allowIncomingConnections</code>); this supplies the shared nodes
          Tailscale would learn from the other tailnet. A node's tags / groups are as its{" "}
          <em>home</em> tailnet assigned them (used to resolve <code>group://</code> /{" "}
          <code>tag://</code>).
        </p>
      </div>
    </Panel>
  );
}

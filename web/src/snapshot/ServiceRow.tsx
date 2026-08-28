import type { Device, Service } from "../engine/types";
import { splitList } from "./splitList";

// One Tailscale Service row: name, VIP, and the backend devices (edited as a
// comma-separated list of device names, stored as device IDs). Feeds
// engine/service — svc:<name> destinations are rewritten to the VIP.
export function ServiceRow({
  service,
  devices,
  onChange,
  onDelete,
}: {
  service: Service;
  devices: Device[];
  onChange: (s: Service) => void;
  onDelete: () => void;
}) {
  const nameById = new Map(devices.map((d) => [d.id, d.name]));
  const idByName = new Map(devices.map((d) => [d.name, d.id]));
  const backendText = (service.backends ?? [])
    .map((id) => nameById.get(id) ?? String(id))
    .join(", ");
  return (
    <tr>
      <td>
        <input
          type="text"
          value={service.name}
          onChange={(e) => onChange({ ...service, name: e.target.value })}
        />
      </td>
      <td>
        <input
          type="text"
          placeholder="100.96.0.10"
          value={service.vip}
          onChange={(e) => onChange({ ...service, vip: e.target.value })}
        />
      </td>
      <td className="tags-cell">
        <input
          type="text"
          placeholder="api-1, api-2"
          value={backendText}
          onChange={(e) => {
            const backends = (splitList(e.target.value) ?? [])
              .map((n) => idByName.get(n))
              .filter((id): id is number => id !== undefined);
            onChange({ ...service, backends: backends.length ? backends : undefined });
          }}
        />
      </td>
      <td>
        <button className="row-del" onClick={onDelete}>
          ✕
        </button>
      </td>
    </tr>
  );
}

import type { Device, Diagnostic } from "../engine/types";
import { DiagMark } from "../ui/DiagMark";
import { RoutesCell } from "./RoutesCell";
import { PostureCell } from "./PostureCell";
import { splitList } from "./splitList";

export function DeviceRow({
  device,
  diags,
  users,
  onChange,
  onDelete,
}: {
  device: Device;
  diags: Diagnostic[];
  users: string[];
  onChange: (d: Device) => void;
  onDelete: () => void;
}) {
  return (
    <tr className={diags.length ? "has-diag" : undefined}>
      <td className="name-cell">
        <input
          type="text"
          value={device.name}
          onChange={(e) => onChange({ ...device, name: e.target.value })}
        />
        <DiagMark diags={diags} />
      </td>
      <td>
        <select
          value={device.user ?? ""}
          onChange={(e) =>
            onChange({ ...device, user: e.target.value || undefined })
          }
        >
          <option value="">— (tagged)</option>
          {users.map((u) => (
            <option key={u} value={u}>
              {u}
            </option>
          ))}
        </select>
      </td>
      <td className="tags-cell">
        <input
          type="text"
          placeholder="tag:web, tag:db"
          value={(device.tags ?? []).join(", ")}
          onChange={(e) =>
            onChange({
              ...device,
              tags: splitList(e.target.value),
            })
          }
        />
      </td>
      <td>
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <input
            type="text"
            placeholder="auto v4"
            value={device.ipv4 ?? ""}
            onChange={(e) =>
              onChange({ ...device, ipv4: e.target.value || undefined })
            }
          />
          <input
            type="text"
            placeholder="auto v6"
            style={{ fontSize: 11 }}
            value={device.ipv6 ?? ""}
            onChange={(e) =>
              onChange({ ...device, ipv6: e.target.value || undefined })
            }
          />
        </div>
      </td>
      <td>
        <input
          type="text"
          placeholder="linux"
          value={device.os ?? ""}
          onChange={(e) => onChange({ ...device, os: e.target.value || undefined })}
        />
      </td>
      <td>
        <RoutesCell device={device} onChange={onChange} />
      </td>
      <td>
        <PostureCell device={device} onChange={onChange} />
      </td>
      <td>
        <button className="row-del" onClick={onDelete}>
          ✕
        </button>
      </td>
    </tr>
  );
}

import type { SnapshotUser } from "../engine/types";

// Tailnet roles the engine's autogroup layer recognises (engine/autogroups).
// "member" is the default and is shown as the empty selection.
const USER_ROLES = [
  "owner",
  "admin",
  "it-admin",
  "network-admin",
  "billing-admin",
  "auditor",
] as const;

export function UserRow({
  user,
  onChange,
  onDelete,
}: {
  user: SnapshotUser;
  onChange: (patch: Partial<SnapshotUser>) => void;
  onDelete: () => void;
}) {
  return (
    <tr>
      <td>
        <input type="text" value={user.email} onChange={(e) => onChange({ email: e.target.value })} />
      </td>
      <td>
        <select
          value={user.role ?? ""}
          onChange={(e) => onChange({ role: e.target.value || undefined })}
        >
          <option value="">member</option>
          {USER_ROLES.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
      </td>
      <td style={{ textAlign: "center" }}>
        <input
          type="checkbox"
          checked={user.shared ?? false}
          onChange={(e) => onChange({ shared: e.target.checked || undefined })}
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

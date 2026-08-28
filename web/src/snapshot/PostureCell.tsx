import { useState } from "react";
import type { Device } from "../engine/types";

// Editor for a device's node:* posture attributes (tsVersion, tsReleaseTrack)
// and any custom:* attributes (declared in the policy's attrConfig). node:os is
// driven by the os column.
export function PostureCell({
  device,
  onChange,
}: {
  device: Device;
  onChange: (d: Device) => void;
}) {
  const [open, setOpen] = useState(false);
  const [newKey, setNewKey] = useState("");
  const p = device.posture ?? {};

  const set = (key: string, value: string) => {
    const next = { ...p };
    if (value) next[key] = value;
    else delete next[key];
    onChange({ ...device, posture: Object.keys(next).length ? next : undefined });
  };

  const rename = (from: string, to: string) => {
    const next = { ...p };
    const v = next[from];
    delete next[from];
    if (to) next[to] = v ?? "";
    onChange({ ...device, posture: Object.keys(next).length ? next : undefined });
  };

  const customKeys = Object.keys(p).filter((k) => k.startsWith("custom:"));
  const summary =
    [p.tsVersion, p.tsReleaseTrack, ...customKeys.map((k) => `${k}=${p[k]}`)]
      .filter(Boolean)
      .join(" · ") || "—";

  return (
    <div style={{ position: "relative" }}>
      <button
        className="pill"
        style={{ width: "100%", fontFamily: "var(--font-mono)" }}
        onClick={() => setOpen((v) => !v)}
      >
        {summary}
      </button>
      {open && (
        <div className="posture-pop">
          <label>
            node:tsVersion
            <input
              type="text"
              placeholder="1.82.0"
              value={p.tsVersion ?? ""}
              onChange={(e) => set("tsVersion", e.target.value.trim())}
            />
          </label>
          <label>
            node:tsReleaseTrack
            <select
              value={p.tsReleaseTrack ?? ""}
              onChange={(e) => set("tsReleaseTrack", e.target.value)}
            >
              <option value="">—</option>
              <option value="stable">stable</option>
              <option value="unstable">unstable</option>
            </select>
          </label>
          <p style={{ fontSize: 10, color: "var(--text-subtle)", margin: "2px 0 0" }}>
            <code>node:os</code> comes from the os column.
          </p>

          <div style={{ marginTop: 6, borderTop: "1px solid var(--border)", paddingTop: 6 }}>
            <div style={{ fontSize: 10, color: "var(--text-subtle)", marginBottom: 2 }}>
              custom attributes (declared in the policy's <code>attrConfig</code>)
            </div>
            {customKeys.map((k) => (
              <div key={k} style={{ display: "flex", gap: 4, marginBottom: 2 }}>
                <input
                  type="text"
                  style={{ flex: "1 1 55%", fontFamily: "var(--font-mono)", fontSize: 11 }}
                  value={k}
                  onChange={(e) => rename(k, e.target.value.trim())}
                />
                <input
                  type="text"
                  style={{ flex: "1 1 45%", fontSize: 11 }}
                  placeholder="value"
                  value={p[k] ?? ""}
                  onChange={(e) => set(k, e.target.value.trim())}
                />
              </div>
            ))}
            <div style={{ display: "flex", gap: 4 }}>
              <input
                type="text"
                style={{ flex: 1, fontFamily: "var(--font-mono)", fontSize: 11 }}
                placeholder="custom:env"
                value={newKey}
                onChange={(e) => setNewKey(e.target.value)}
              />
              <button
                className="pill"
                style={{ fontSize: 11 }}
                onClick={() => {
                  const k = newKey.trim();
                  if (k) {
                    set(k.startsWith("custom:") ? k : `custom:${k}`, "");
                    setNewKey("");
                  }
                }}
              >
                +
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

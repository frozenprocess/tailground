import { useState } from "react";
import type { Device } from "../engine/types";
import { splitList } from "./splitList";

// Editor for a device's advertised subnet / exit routes. A route must be both
// advertised and admin-approved to carry traffic; the popover lets you toggle
// approval per prefix so the "advertised, pending approval" state is authorable.
// A freshly typed prefix defaults to approved. "0.0.0.0/0" / "::/0" make the
// device an exit node.
export function RoutesCell({
  device,
  onChange,
}: {
  device: Device;
  onChange: (d: Device) => void;
}) {
  const [open, setOpen] = useState(false);
  const routes = device.routes ?? [];
  const approved = new Set(device.routesApproved ?? []);

  const setRoutes = (next: string[] | undefined) => {
    const list = next ?? [];
    // Keep approval for prefixes that survived; default new ones to approved.
    const nextApproved = list.filter((r) => !routes.includes(r) || approved.has(r));
    onChange({
      ...device,
      routes: next,
      routesApproved: nextApproved.length ? nextApproved : undefined,
    });
  };

  const toggle = (prefix: string) => {
    const next = new Set(approved);
    if (next.has(prefix)) next.delete(prefix);
    else next.add(prefix);
    const list = routes.filter((r) => next.has(r));
    onChange({ ...device, routesApproved: list.length ? list : undefined });
  };

  const pending = routes.filter((r) => !approved.has(r)).length;
  const summary =
    routes.length === 0
      ? "—"
      : routes.join(", ") + (pending ? ` · ${pending} pending` : "");

  return (
    <div style={{ position: "relative" }}>
      <button
        className="pill"
        style={{ width: "100%", fontFamily: "var(--font-mono)" }}
        onClick={() => setOpen((v) => !v)}
        title={summary}
      >
        {summary}
      </button>
      {open && (
        <div className="posture-pop" style={{ width: 240 }}>
          <label>
            advertised routes
            <input
              type="text"
              placeholder="10.0.0.0/24, 0.0.0.0/0"
              value={routes.join(", ")}
              onChange={(e) => setRoutes(splitList(e.target.value))}
            />
          </label>
          {routes.map((r) => (
            <label
              key={r}
              style={{ flexDirection: "row", gap: 6, alignItems: "center" }}
            >
              <input
                type="checkbox"
                checked={approved.has(r)}
                onChange={() => toggle(r)}
              />
              <code style={{ fontSize: 11 }}>{r}</code>
              {!approved.has(r) && (
                <span style={{ fontSize: 10, color: "var(--text-subtle)" }}>pending</span>
              )}
            </label>
          ))}
          <p style={{ fontSize: 10, color: "var(--text-subtle)", margin: "2px 0 0" }}>
            A route carries traffic only when advertised <em>and</em> approved.
          </p>
        </div>
      )}
    </div>
  );
}

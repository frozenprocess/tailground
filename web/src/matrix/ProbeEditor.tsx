import { useState } from "react";
import { useStore } from "../state/store";
import type { Probe } from "../engine/types";

// Protocols that carry L4 ports. Everything else is a bare protocol probe.
const PORT_PROTOS = new Set(["tcp", "udp", "sctp", "dccp"]);
// Port-less protocols we accept by name (mirrors engine `protoOf` / tailscale's
// ipproto). Raw IANA numbers are handled via the "ip:<n>" form.
const PROTO_NAMES = new Set([
  "icmp", "icmpv6", "igmp", "gre", "esp", "ah", "egp", "igp", "ipv4", "ip-in-ip",
  "ipip", "tsmp",
]);

/**
 * Parse a probe spec:
 *   "443" / "tcp:443" / "udp:53" / "sctp:9899"   L4 port probes
 *   "icmp" / "icmpv6" / "gre" / "esp" / "ipv4"   bare protocol probes
 *   "ip:47" / "ip:112"                           raw IANA protocol number 0-255
 */
function parseProbe(input: string): Probe | null {
  const s = input.trim().toLowerCase();
  if (!s) return null;

  const raw = s.match(/^(?:ip|proto):(\d{1,3})$/);
  if (raw) {
    const n = parseInt(raw[1], 10);
    if (n < 0 || n > 255) return null;
    return { name: `ip:${n}`, proto: String(n) };
  }

  const withPort = s.match(/^([a-z][a-z-]*):(\d{1,5})$/);
  if (withPort) {
    const proto = withPort[1];
    const port = parseInt(withPort[2], 10);
    if (!PORT_PROTOS.has(proto) || port < 1 || port > 65535) return null;
    return { name: `${proto}:${port}`, proto, port };
  }

  if (/^\d{1,5}$/.test(s)) {
    const port = parseInt(s, 10);
    if (port < 1 || port > 65535) return null;
    return { name: `tcp:${port}`, proto: "tcp", port };
  }

  if (PROTO_NAMES.has(s)) return { name: s, proto: s };

  return null;
}

export function ProbeEditor() {
  const probes = useStore((s) => s.probes);
  const setProbes = useStore((s) => s.setProbes);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [err, setErr] = useState(false);

  const add = () => {
    const p = parseProbe(draft);
    if (!p) {
      setErr(true);
      return;
    }
    if (!probes.some((x) => x.name === p.name)) setProbes([...probes, p]);
    setDraft("");
    setErr(false);
  };

  return (
    <div style={{ position: "relative" }}>
      <button className="btn" onClick={() => setOpen((v) => !v)} title="Edit probes">
        probes ⋯
      </button>
      {open && (
        <div
          style={{
            position: "absolute",
            right: 0,
            top: "calc(100% + 4px)",
            zIndex: 10,
            background: "var(--surface)",
            border: "1px solid var(--border-strong)",
            borderRadius: "var(--radius-sm)",
            boxShadow: "var(--shadow-md)",
            padding: 10,
            width: 220,
          }}
        >
          <div style={{ display: "flex", flexDirection: "column", gap: 4, marginBottom: 8 }}>
            {probes.map((p) => (
              <div
                key={p.name}
                style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}
              >
                <code style={{ fontSize: 12 }}>{p.name}</code>
                <button
                  className="row-del"
                  onClick={() => setProbes(probes.filter((x) => x.name !== p.name))}
                  disabled={probes.length === 1}
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
          <div style={{ display: "flex", gap: 4 }}>
            <input
              type="text"
              value={draft}
              placeholder="tcp:8080"
              style={{ flex: 1, borderColor: err ? "var(--accent)" : undefined }}
              onChange={(e) => {
                setDraft(e.target.value);
                setErr(false);
              }}
              onKeyDown={(e) => e.key === "Enter" && add()}
            />
            <button className="btn" onClick={add}>
              add
            </button>
          </div>
          <p
            style={{
              margin: "6px 0 0",
              fontSize: 11,
              lineHeight: 1.4,
              color: "var(--text-subtle)",
            }}
          >
            <code>tcp:443</code>, <code>udp:53</code>, <code>sctp:9899</code>,{" "}
            <code>icmp</code>, <code>gre</code>, <code>esp</code>,{" "}
            <code>ipv4</code> (IP-in-IP), or a raw number as <code>ip:47</code>.
          </p>
        </div>
      )}
    </div>
  );
}

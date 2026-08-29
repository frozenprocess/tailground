import type { EngineResult } from "../engine/types";

/** Prefixes that mean "the public internet" when they appear as a filter dst. */
function isInternetPrefix(portSpec: string): boolean {
  const p = portSpec.split(":")[0];
  return p === "0.0.0.0/0" || p === "::/0";
}

/** Parse a dotted IPv4 literal to a uint32, or null if it isn't one. */
function ipv4ToInt(ip: string): number | null {
  const m = ip.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/);
  if (!m) return null;
  const o = m.slice(1).map(Number);
  if (o.some((n) => n > 255)) return null;
  return ((o[0] << 24) | (o[1] << 16) | (o[2] << 8) | o[3]) >>> 0;
}

/**
 * Does a filter-rule source term cover this device IP? Handles bare addresses,
 * `*`, the internet supernets, and IPv4 CIDRs — an exit-node rule's source is
 * often a CIDR (or `*`), which the old exact-match logic dropped, silently
 * hiding the internet-egress edge for everyone behind it.
 */
function srcCoversIP(src: string, ip: string): boolean {
  if (src === ip) return true;
  if (src === "*") return true;
  if (src === "0.0.0.0/0") return ip.includes(".");
  if (src === "::/0") return ip.includes(":");
  const slash = src.indexOf("/");
  if (slash === -1) return src === ip.replace(/\/(32|128)$/, "");
  const net = ipv4ToInt(src.slice(0, slash));
  const host = ipv4ToInt(ip);
  const bits = Number(src.slice(slash + 1));
  if (net === null || host === null || Number.isNaN(bits)) return false;
  if (bits <= 0) return true;
  if (bits >= 32) return net === host;
  const mask = (~0 << (32 - bits)) >>> 0;
  return (net & mask) === (host & mask);
}

export function internetEgress(result: EngineResult): Set<number> {
  const out = new Set<number>();
  for (const rules of Object.values(result.rules ?? {})) {
    for (const r of rules) {
      if (!(r.dstPorts ?? []).some(isInternetPrefix)) continue;
      for (const srcIP of r.srcIPs) {
        for (const a of result.actors) {
          if (a.ips.some((ip) => srcCoversIP(srcIP, ip))) out.add(a.id);
        }
      }
    }
  }
  return out;
}

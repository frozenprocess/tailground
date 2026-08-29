import { Fragment, type ReactNode } from "react";
import { useStore } from "../state/store";
import type { Capability, CapabilityGroup } from "../engine/types";

/** Display order and static prose for each manifest group. The rows themselves
 *  come from the engine (engine/eval/capabilities.go); only the framing lives here. */
const GROUPS: { id: CapabilityGroup; title: string; intro?: ReactNode; outro?: ReactNode }[] = [
  { id: "headscale", title: "Evaluated by the Headscale engine" },
  {
    id: "tailground",
    title: "Evaluated by Tailground's own layer",
    outro: (
      <>
        Each posture-gated rule is compiled on its own and applied per source
        device. Verdicts it changes are labelled as Tailground's interpretation
        of the syntax docs, and shown in the posture panel. An end-to-end harness
        confirms the built-in conditions on <code>acls</code>, <code>grants</code>{" "}
        <em>and</em> <code>ssh</code> rules match a real Tailscale tailnet
        exactly. On <code>ssh</code> rules the list is evaluated in order and a{" "}
        <code>srcPosture</code> miss falls through to the next rule, the way
        Tailscale's control plane resolves it.
      </>
    ),
  },
  {
    id: "expanded",
    title: "Expanded before compilation",
    intro:
      "Headscale doesn't understand these, so Tailground rewrites them into primitives it does, then compiles normally:",
  },
  {
    id: "not-modeled",
    title: "Not modeled",
    intro: (
      <>
        The preflight pass strips these before compiling instead of failing the
        parse. The editor flags one only when it can change a verdict (e.g.{" "}
        <code>postureIntegrations</code>); all are listed here:
      </>
    ),
  },
  {
    id: "by-design",
    title: "Intentionally not modeled",
    intro:
      "These configure relays, address assignment, client route generation and DNS names — none of them change which connections a policy allows, so Tailground strips them silently by design, not as a gap:",
  },
];

/** Renders `backtick spans` in manifest text as <code>. Everything else is literal. */
function Inline({ text }: { text: string }) {
  return (
    <>
      {text.split("`").map((part, i) =>
        i % 2 ? <code key={i}>{part}</code> : <Fragment key={i}>{part}</Fragment>,
      )}
    </>
  );
}

const BUILD_INFO = {
  // Injected by Vite at build time (see vite.config.ts).
  date: (import.meta.env.VITE_BUILD_DATE as string | undefined) ?? "",
};

/** Body of the About modal (the dialog chrome lives in AboutButton). */
export function AboutEngine() {
  const engine = useStore((s) => s.result?.engine);
  const status = useStore((s) => s.engineStatus);
  const caps: Capability[] | undefined = engine?.capabilities;
  const pending = status === "error" ? "(engine failed to load)" : "(loading…)";

  return (
    <div className="pane-body">
      <div className="about">
        <div>
          <div className="eyebrow">How Tailground evaluates your policy</div>
          <h1>A real compiler, in your browser</h1>
          <p>
            Tailground evaluates policy files with{" "}
            <a href="https://github.com/juanfont/headscale" target="_blank" rel="noreferrer">
              Headscale
            </a>
            's <code>hscontrol/policy/v2</code> package — an open-source (BSD-3)
            reimplementation of Tailscale's closed-source ACL compiler — together
            with Tailscale's own <code>wgengine/filter</code> packet matcher. Both
            are compiled to WebAssembly and run entirely on this page. Your scenario
            is evaluated in your browser and is only sent to a server if you create a
            short share link.
          </p>
          <p className="quote">
            Headscale's policy engine targets Tailscale parity but is not
            Tailscale's code. Where a captured fixture and Tailscale's SaaS disagree
            in an untested corner, Tailground shows the Headscale behaviour.
          </p>
        </div>

        <div className="card">
          <h3>Pinned build</h3>
          <p className="provenance">
            headscale&nbsp;
            {engine?.headscaleCommit ? (
              <a
                href={`https://github.com/juanfont/headscale/commit/${engine.headscaleCommit}`}
                target="_blank"
                rel="noreferrer"
              >
                {engine.headscaleCommit.slice(0, 12)}
              </a>
            ) : (
              pending
            )}
            <br />
            tailscale&nbsp;{engine?.tailscaleVersion || pending}
            {BUILD_INFO.date && (
              <>
                <br />
                built&nbsp;{BUILD_INFO.date}
              </>
            )}
          </p>
        </div>

        <div className="card">
          <h3>Sharing &amp; privacy</h3>
          <p className="card-note flush">
            "Share" packs the whole scenario — policy, snapshot, probes, the
            active view — into a compressed token. A long link carries that token in
            the URL fragment and never reaches a server. A short link stores a copy
            on the Tailground server for 1 year after it was last opened, readable by
            anyone with the link. There are no accounts; otherwise nothing is
            persisted, and reload gives you the defaults back.
          </p>
        </div>

        {!caps && (
          <div className="card">
            <h3>Capabilities</h3>
            <p className="provenance" role="status">
              {status === "error"
                ? "The engine failed to load, so its capability list isn't available. Reload to retry."
                : "Loading the engine's capability list…"}
            </p>
          </div>
        )}

        {caps &&
          GROUPS.map((g) => {
            const rows = caps.filter((c) => c.group === g.id);
            if (!rows.length) return null;
            return (
              <div className="card" key={g.id}>
                <h3>{g.title}</h3>
                {g.intro && <p className="card-note">{g.intro}</p>}
                <ul>
                  {rows.map((c) => (
                    <li key={c.id}>
                      <Inline text={c.name} />
                      {c.notes && (
                        <>
                          {" — "}
                          <Inline text={c.notes} />
                        </>
                      )}
                    </li>
                  ))}
                </ul>
                {g.outro && <p className="card-note subtle">{g.outro}</p>}
              </div>
            );
          })}
      </div>
    </div>
  );
}

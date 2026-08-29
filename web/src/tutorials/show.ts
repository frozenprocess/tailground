import { useStore, type MobilePane } from "../state/store";

// "Show me" actions for lessons. A lesson writes `[Show me](show:matrix)`; the
// button drives the real app — switches pane, sets the probe/selection — and
// pulses the part it's talking about, instead of embedding a picture of it.
// Targets are looked up after the state change, since panes mount lazily.

interface Action {
  /** Pane to bring forward (mobile shows one at a time; desktop mostly already has it). */
  pane?: MobilePane;
  /** Store changes to make. */
  run?: () => void;
  /** CSS selectors (or a finder) for what to pulse. */
  targets: string[] | (() => Element | null);
  /** Clicks a button instead of / as well as pulsing. */
  click?: string;
}

const st = () => useStore.getState();
const actorId = (name: string) => st().result?.actors.find((a) => a.name === name)?.id ?? null;

const cell = (src: string, dst: string): Element | null => {
  const t = document.querySelector("table.matrix");
  if (!t) return null;
  const head = t.querySelector("thead tr");
  const col = head ? [...head.children].findIndex((h) => h.textContent?.includes(dst)) : -1;
  const row = [...t.querySelectorAll("tbody tr")].find((r) => r.children[0]?.textContent?.includes(src));
  return col >= 0 && row ? (row.children[col] ?? null) : null;
};

const ACTIONS: Record<string, Action> = {
  layout: { targets: ["[data-panel=input]", "[data-panel=editor]", "[data-panel=impact]", ".mobile-nav"] },
  editor: { pane: "policy", targets: ["[data-panel=editor]"] },
  tailnet: { pane: "tailnet", targets: ["[data-panel=input]"] },
  matrix: {
    pane: "matrix",
    run: () => {
      const { src, dst } = { src: actorId("carol-laptop"), dst: actorId("web-1") };
      st().setActiveProbe("https");
      st().setFocusedActor(null);
      if (src != null && dst != null) st().selectPair({ src, dst });
    },
    targets: () => cell("carol-laptop", "web-1"),
  },
  ssh: {
    pane: "matrix",
    run: () => {
      st().selectPair(null);
      st().setFocusedActor(null);
      st().setActiveProbe("ssh:ubuntu");
    },
    targets: ["table.matrix"],
  },
  inspector: {
    pane: "matrix",
    run: () => {
      st().setActiveProbe("https");
      st().selectPair(null);
      st().setFocusedActor(actorId("web-1"));
    },
    targets: [".device-inspector"],
  },
  graph: {
    pane: "graph",
    run: () => {
      st().setActiveProbe("https");
      st().setGraphAnyProbe(false);
      st().setGraphHighlightGroup("group:eng");
      st().setFocusedActor(actorId("web-1")); // pinned, so its flows are already traced
    },
    // The graph chunk is lazy: wait for the canvas, not the placeholder panel.
    targets: () => (document.querySelector(".react-flow") ? document.querySelector("[data-panel=impact]") : null),
  },
  learn: { pane: "learn", targets: [".lesson-footer"] },
  share: { targets: [".share > .btn"], click: ".share > .btn" },
  theme: { targets: [".theme-toggle"] },
  about: { targets: [".about-btn"] },
};

async function find(get: () => Element | null, ms = 2500): Promise<Element | null> {
  const end = Date.now() + ms;
  for (;;) {
    const el = get();
    if (el) return el;
    if (Date.now() > end) return null;
    await new Promise((r) => setTimeout(r, 60));
  }
}

function pulse(el: Element) {
  el.scrollIntoView({ block: "nearest", inline: "nearest", behavior: "smooth" });
  el.classList.remove("tour-pulse");
  void (el as HTMLElement).offsetWidth; // restart the animation
  el.classList.add("tour-pulse");
  window.setTimeout(() => el.classList.remove("tour-pulse"), 2800);
}

export async function runShow(id: string): Promise<void> {
  const a = ACTIONS[id];
  if (!a) return;
  const s = st();
  if (s.layout.maximized) s.toggleMaximized(s.layout.maximized);
  if (a.pane) s.setMobilePane(a.pane);
  a.run?.();
  if (a.pane && a.pane !== "learn") s.setTourNote(true);

  if (a.click) {
    const b = await find(() => document.querySelector(a.click!));
    (b as HTMLElement | null)?.click();
  }
  // Let React commit the new pane (and any lazy remount) before marking it.
  await new Promise((r) => setTimeout(r, 300));
  const sels = a.targets;
  const els = typeof sels === "function" ? [await find(sels)] : await Promise.all(sels.map((q) => find(() => document.querySelector(q))));
  for (const el of els) if (el) pulse(el);
}

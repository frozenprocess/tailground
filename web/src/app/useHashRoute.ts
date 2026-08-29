import { useEffect, useRef } from "react";
import { canonicalView, useStore, type MainView } from "../state/store";

// Two-way sync between the URL fragment and {view, activeLesson}. Deep links:
//   (no fragment)          Learn — the default landing
//   #/                     playground
//   #/graph  #/about       those views
//   #/tutorials            the lesson index
//   #/tutorials/<slug>     one lesson
export function parseHash(hash: string): { view: MainView; lesson: string | null } {
  // A bare URL with no fragment lands on the lesson index; "#/" is the explicit
  // playground route.
  if (hash === "") return { view: "tutorials", lesson: null };
  const path = hash.replace(/^#\/?/, "");
  if (path.startsWith("tutorials")) {
    const slug = path.split("/")[1] || null;
    return { view: "tutorials", lesson: slug };
  }
  if (path === "graph") return { view: "graph", lesson: null };
  if (path === "about") return { view: "about", lesson: null };
  return { view: "matrix", lesson: null };
}

export function toHash(view: MainView, lesson: string | null): string {
  switch (view) {
    case "tutorials":
      return lesson ? `#/tutorials/${lesson}` : "#/tutorials";
    case "graph":
      return "#/graph";
    case "about":
      return "#/about";
    default:
      return "#/";
  }
}

export function useHashRoute() {
  const view = useStore((s) => s.view);
  const leftTab = useStore((s) => s.leftTab);
  const activeLesson = useStore((s) => s.activeLesson);
  const applyRoute = useStore((s) => s.applyRoute);
  const applying = useRef(false);
  // The store→hash mirror must stay quiet until the initial URL has been applied
  // to the store. On mount the mirror effect's closure still holds the default
  // view, so firing it early rewrites `#/graph` → `#/` before the state catches
  // up (and StrictMode's re-run then re-reads the clobbered URL). It only needs
  // to run for user-driven changes, which all come after this settles.
  const mirrorReady = useRef(false);

  // Hash → store.
  useEffect(() => {
    const apply = () => {
      const { view, lesson } = parseHash(window.location.hash);
      applying.current = true;
      applyRoute(view, lesson);
      applying.current = false;
    };
    apply();
    window.addEventListener("hashchange", apply);
    return () => window.removeEventListener("hashchange", apply);
  }, [applyRoute]);

  useEffect(() => {
    const id = window.setTimeout(() => {
      mirrorReady.current = true;
    }, 0);
    return () => window.clearTimeout(id);
  }, []);

  // Store → hash. Both UI axes feed the canonical view the URL carries.
  useEffect(() => {
    if (applying.current || !mirrorReady.current) return;
    const want = toHash(canonicalView({ leftTab, view }), activeLesson);
    if (window.location.hash !== want) {
      window.history.replaceState(null, "", want);
    }
  }, [view, leftTab, activeLesson]);
}

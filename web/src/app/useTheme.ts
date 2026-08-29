import { useEffect } from "react";
import { useStore } from "../state/store";

/**
 * Reflects the store's theme preference onto <html data-theme> (which theme.css
 * keys the dark palette off) and, while on "system", re-derives it when the OS
 * theme changes. Mount once near the app root.
 *
 * The initial data-theme is set by the inline script in index.html to avoid a
 * flash; this keeps it in sync for the rest of the session.
 */
export function useThemeSync(): void {
  const themePref = useStore((s) => s.themePref);
  const themeResolved = useStore((s) => s.themeResolved);
  const syncResolvedTheme = useStore((s) => s.syncResolvedTheme);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", themeResolved);
  }, [themeResolved]);

  useEffect(() => {
    if (themePref !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", syncResolvedTheme);
    return () => mq.removeEventListener("change", syncResolvedTheme);
  }, [themePref, syncResolvedTheme]);
}

import { useEffect, useState } from "react";

/**
 * Subscribe to a CSS media query. SSR-safe (returns `false` until mounted) and
 * updates live on viewport / orientation changes.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => {
    try {
      return window.matchMedia(query).matches;
    } catch {
      return false;
    }
  });

  useEffect(() => {
    let mq: MediaQueryList;
    try {
      mq = window.matchMedia(query);
    } catch {
      return;
    }
    const onChange = () => setMatches(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [query]);

  return matches;
}

/** The single breakpoint below which the app uses the stacked mobile layout. */
export const MOBILE_QUERY = "(max-width: 768px)";

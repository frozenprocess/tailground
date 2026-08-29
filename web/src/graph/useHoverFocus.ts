import { useEffect, useRef, useState } from "react";
import type { PairSelection } from "../state/store";

/** How long the pointer may be off every node / edge before the hover clears. */
const HOVER_CLEAR_MS = 140;

export const samePair = (x: PairSelection | null, y: PairSelection | null) =>
  x === y || (!!x && !!y && x.src === y.src && x.dst === y.dst);

/**
 * Hover-to-focus: the node / device pair under the pointer, previewed until a
 * click pins it in the store.
 *
 * Focusing a node rebuilds the edge set; if a fresh edge lands under a
 * stationary cursor the browser fires enter/leave against it, which would fight
 * the node hover and strobe the graph. Two guards: (1) while a focus is active
 * the edge layer drops `pointer-events` (CSS `.node-focus`), so events pass
 * straight through to the node; (2) the hover *clear* is debounced
 * (scheduleClear), so a brief slip between a node and its own edge doesn't drop
 * the focus.
 */
export function useHoverFocus() {
  const [hoverActor, setHoverActor] = useState<number | null>(null);
  const [hoverPair, setHoverPair] = useState<PairSelection | null>(null);

  const timer = useRef<number | null>(null);
  const cancelClear = () => {
    if (timer.current != null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  };
  const scheduleClear = () => {
    cancelClear();
    timer.current = window.setTimeout(() => {
      timer.current = null;
      setHoverActor(null);
      setHoverPair(null);
    }, HOVER_CLEAR_MS);
  };
  useEffect(() => cancelClear, []);

  return {
    hoverActor,
    hoverPair,
    cancelClear,
    scheduleClear,
    /** The pointer is on a focusable node. */
    hoverNode(actor: number) {
      cancelClear();
      setHoverActor((p) => (p === actor ? p : actor));
      setHoverPair(null);
    },
    /** The pointer is on a device↔device edge. */
    hoverEdge(pair: PairSelection) {
      setHoverPair((prev) => (samePair(prev, pair) ? prev : pair));
    },
    clear() {
      setHoverActor(null);
      setHoverPair(null);
    },
  };
}

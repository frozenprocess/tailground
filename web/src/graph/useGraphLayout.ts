import { useEffect, useState } from "react";
import { layoutGraph, type Positioned } from "./layout";
import type { GraphModel } from "./model";

/**
 * Runs the async ELK layout whenever the model's signature changes.
 * `ready` is true only while `positioned` was computed for the *current*
 * model: node and edge builds gate on it, so RF never sees a half-swapped graph
 * (new edges against old nodes, or nodes piled at the origin) while a relayout
 * is in flight — that mismatch made RF silently drop edges on regroup,
 * especially on slower mobile renders.
 */
export function useGraphLayout(model: GraphModel | null) {
  const [positioned, setPositioned] = useState<Positioned | null>(null);
  // The signature the current `positioned` was computed for.
  const [layoutSig, setLayoutSig] = useState<string>("");
  const [laying, setLaying] = useState(false);

  useEffect(() => {
    if (!model) return;
    if (layoutSig === model.signature && positioned) return;
    let alive = true;
    setLaying(true);
    layoutGraph(model).then((p) => {
      if (!alive) return;
      setPositioned(p);
      setLayoutSig(model.signature);
      setLaying(false);
    });
    return () => {
      alive = false;
    };
  }, [model, positioned, layoutSig]);

  const ready = !!model && !!positioned && layoutSig === model.signature;
  return { positioned, layoutSig, laying, ready };
}

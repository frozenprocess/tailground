import { useEffect, useRef } from "react";
import { getEngineClient } from "../engine/client";
import type { EngineRequest } from "../engine/types";
import { useStore } from "../state/store";

const DEBOUNCE_MS = 200;

/** Page-lifetime worker, with debounced publication of the latest input state. */
export function useEvaluation() {
  const client = getEngineClient();
  const timer = useRef<number | undefined>(undefined);
  const policy = useStore((s) => s.policy);
  const snapshot = useStore((s) => s.snapshot);
  const probes = useStore((s) => s.probes);
  const sshUsers = useStore((s) => s.sshUsers);
  const setResult = useStore((s) => s.setResult);
  const setEngineStatus = useStore((s) => s.setEngineStatus);
  const setEvaluating = useStore((s) => s.setEvaluating);

  // Engine lifecycle. The worker is a page-lifetime singleton; no teardown.
  useEffect(() => {
    client.onStatusChange = setEngineStatus;
    setEngineStatus(client.getStatus());
  }, [client, setEngineStatus]);

  // Debounced evaluate on any input change (once the engine is up).
  useEffect(() => {
    let cancelled = false;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(async () => {
      const status = await client.whenSettled();
      if (cancelled || status !== "ready") return;
      const request: EngineRequest = { policy, snapshot, probes, sshUsers };
      setEvaluating(true);
      const { result, ms } = await client.evaluate(request);
      if (!cancelled) setResult(result, ms);
    }, DEBOUNCE_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer.current);
    };
  }, [policy, snapshot, probes, sshUsers, client, setResult, setEvaluating]);
}

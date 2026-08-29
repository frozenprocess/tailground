import { lazy, Suspense } from "react";
import { useStore } from "../state/store";
import { SnapshotPanel } from "../snapshot/SnapshotPanel";
import { Panel } from "../ui/Panel";
import { LeftTabs } from "./LeftTabs";

// LessonPane pulls in react-markdown + the lesson bodies; only load that chunk
// when the Learn tab is first opened.
const LearnPanel = lazy(() =>
  import("../learn/LearnPanel").then((m) => ({ default: m.LearnPanel })),
);

/**
 * The left workspace column: one panel that switches between the tailnet
 * snapshot and the guided lessons.
 * `leftTab` is the only input —
 * independent of the policy editor and the right panel's Matrix/Graph lens.
 */
export function LeftPanel() {
  const leftTab = useStore((s) => s.leftTab);

  if (leftTab === "learn") {
    return (
      <Suspense
        fallback={
          <Panel id="input" title="Learn" tabs={<LeftTabs />}>
            <p className="empty">Loading lessons…</p>
          </Panel>
        }
      >
        <LearnPanel />
      </Suspense>
    );
  }
  return <SnapshotPanel />;
}

import { Panel } from "../ui/Panel";
import { usePaneMax } from "../app/usePaneMax";
import { LeftTabs } from "../app/LeftTabs";
import { LessonPane } from "../tutorials/LessonPane";

/** The Learn lens of the left panel (L-E): the guided lessons. */
export function LearnPanel() {
  return (
    <Panel id="input" title="Learn" tabs={<LeftTabs />} {...usePaneMax("input")}>
      <LessonPane />
    </Panel>
  );
}

import type { TestGroup, TestReport } from "../engine/types";

/** Live pass/fail of the lesson's `tests` / `sshTests` assertions. */
export function GradingChecklist({
  report,
  complete,
  graded,
}: {
  report: TestReport | undefined;
  complete: boolean;
  graded: boolean;
}) {
  const groups: [string, TestGroup][] = [];
  if (report?.acl) groups.push(["Reachability", report.acl]);
  if (report?.ssh) groups.push(["SSH", report.ssh]);

  if (groups.length === 0) {
    return (
      <div className="grading empty-grading">
        {graded
          ? "Waiting for a valid policy — fix the error flagged in the editor."
          : "This lesson has no automated checks — follow the steps and watch the matrix."}
      </div>
    );
  }

  const total = groups.reduce((n, [, g]) => n + g.cases.length, 0);
  const passed = groups.reduce(
    (n, [, g]) => n + g.cases.filter((c) => c.pass).length,
    0,
  );

  return (
    <div className={`grading${complete ? " done" : ""}`}>
      <div className="grading-head">
        <span className="grading-score">
          {complete ? "✓ Lesson complete" : `${passed} / ${total} checks passing`}
        </span>
      </div>
      {groups.map(([name, g]) => (
        <div key={name} className="grading-group">
          {groups.length > 1 && <div className="eyebrow">{name}</div>}
          <ul>
            {g.cases.map((c, i) => (
              <li key={i} className={c.pass ? "ok" : "fail"}>
                <span className="mark">{c.pass ? "✓" : "✗"}</span>
                <span className="case-label">
                  {c.label} <span className="expect">— {c.expect}</span>
                </span>
                {c.detail && <span className="case-detail">{c.detail}</span>}
              </li>
            ))}
          </ul>
          {g.rawFailures?.map((f, i) => (
            <div key={i} className="raw-failure">
              {f}
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

import { useEffect, useMemo, useRef } from "react";
import { useStore } from "../state/store";
import { defaultProbes, type TestReport } from "../engine/types";
import { adjacentLessons, LESSONS, lessonBySlug } from "./lessons";
import { LessonMarkdown } from "./LessonMarkdown";
import { GradingChecklist } from "./GradingChecklist";
import { isFatal } from "../engine/diagnostics";

/** Whether a lesson's scenario ships automated checks. */
function expectsGrading(policy: string): boolean {
  return /"(tests|sshTests)"\s*:/.test(policy);
}

/**
 * A lesson is "complete" when its checks are all green, or — for a
 * reference/explore lesson — once the engine has produced a clean result.
 * A parse/internal error is never "complete".
 */
function lessonComplete(
  hasResult: boolean,
  graded: boolean,
  tests: TestReport | undefined,
  hasFatalError: boolean,
): boolean {
  if (!hasResult || hasFatalError) return false;
  if (graded) {
    if (!tests) return false; // checks expected but gone — policy is broken
    return (!tests.acl || tests.acl.passed) && (!tests.ssh || tests.ssh.passed);
  }
  return true;
}

export function LessonPane() {
  const slug = useStore((s) => s.activeLesson);
  const setActiveLesson = useStore((s) => s.setActiveLesson);
  const setPolicy = useStore((s) => s.setPolicy);
  const setSnapshot = useStore((s) => s.setSnapshot);
  const setProbes = useStore((s) => s.setProbes);
  const result = useStore((s) => s.result);
  const completed = useStore((s) => s.completedLessons);
  const markComplete = useStore((s) => s.markLessonComplete);

  const lesson = lessonBySlug(slug);
  const loadedFor = useRef<string | null>(null);

  const loadScenario = (s: string) => {
    const l = lessonBySlug(s);
    if (!l) return;
    setPolicy(l.scenario.policy);
    setSnapshot(l.scenario.snapshot);
    setProbes(l.scenario.probes ?? defaultProbes());
  };

  // Load the scenario when the lesson changes (not on every edit). `settled`
  // gates completion so a stale result from the previous lesson can't leak a ✓.
  const settled = useRef(false);
  useEffect(() => {
    if (slug && loadedFor.current !== slug) {
      loadedFor.current = slug;
      settled.current = false;
      loadScenario(slug);
      const t = setTimeout(() => (settled.current = true), 600);
      return () => clearTimeout(t);
    }
    settled.current = true;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [slug]);

  const graded = !!lesson && expectsGrading(lesson.scenario.policy);
  const hasFatalError = !!result?.diagnostics?.some(isFatal);
  const complete = useMemo(
    () => lessonComplete(!!result, graded, result?.tests, hasFatalError),
    [result, graded, hasFatalError],
  );

  // Record completion once the freshly-loaded scenario has been evaluated.
  useEffect(() => {
    if (slug && complete && settled.current) markComplete(slug);
  }, [slug, complete, markComplete]);

  const { prev, next } = slug ? adjacentLessons(slug) : {};

  if (!lesson) {
    return (
        <div className="pane-body lesson-index">
          <p className="lesson-intro">
            A guided track through the Tailscale policy syntax. Each lesson loads
            a scenario into the editor; your edits are graded live against the
            policy's own <code>tests</code> block.
          </p>
          <ol>
            {LESSONS.map((l) => (
              <li key={l.slug}>
                <button className="lesson-link" onClick={() => setActiveLesson(l.slug)}>
                  {completed.has(l.slug) ? "✓ " : ""}
                  {l.title}
                </button>
                <span className="lesson-summary">{l.summary}</span>
              </li>
            ))}
          </ol>
        </div>
    );
  }

  const idx = LESSONS.findIndex((l) => l.slug === slug);

  return (
    <>
      <div className="pane-head lesson-head">
        <button className="btn" onClick={() => setActiveLesson(null)} title="All lessons">
          ☰
        </button>
        <select
          value={slug ?? ""}
          onChange={(e) => setActiveLesson(e.target.value)}
        >
          {LESSONS.map((l, i) => (
            <option key={l.slug} value={l.slug}>
              {i + 1}. {l.title}
            </option>
          ))}
        </select>
        <span className="spacer" />
        <span className="eyebrow">
          {idx + 1} / {LESSONS.length}
        </span>
      </div>

      <div className="pane-body lesson-body">
        <div className="eyebrow lesson-track">{lesson.track}</div>
        <h1 className="lesson-title">{lesson.title}</h1>
        <LessonMarkdown body={lesson.body} />
        <button
          className="btn reset-scenario"
          onClick={() => slug && loadScenario(slug)}
        >
          ↻ Reset scenario
        </button>
      </div>

      <div className="lesson-footer">
        <GradingChecklist report={result?.tests} complete={complete} graded={graded} />
        <div className="lesson-nav">
          <button
            className="btn"
            disabled={!prev}
            onClick={() => prev && setActiveLesson(prev.slug)}
          >
            ← {prev?.title ?? "Start"}
          </button>
          <button
            className={`btn${complete && next ? " btn-primary" : ""}`}
            disabled={!next}
            onClick={() => next && setActiveLesson(next.slug)}
          >
            {next?.title ?? "Done"} →
          </button>
        </div>
      </div>
    </>
  );
}

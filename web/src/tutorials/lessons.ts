// Loads the tutorial track from web/src/content/tutorials/<NN-slug>/. Each
// lesson is a directory with meta.json, lesson.md and scenario.json, bundled at
// build time via import.meta.glob.

import type { Probe, Snapshot } from "../engine/types";

interface LessonMeta {
  title: string;
  summary: string;
  track: string;
}

export interface LessonScenario {
  policy: string;
  snapshot: Snapshot;
  probes?: Probe[];
}

export interface Lesson {
  slug: string; // "02-acls"
  order: number; // from the directory prefix
  title: string;
  summary: string;
  track: string;
  body: string; // lesson.md
  scenario: LessonScenario;
}

const metas = import.meta.glob<LessonMeta>("../content/tutorials/*/meta.json", {
  eager: true,
  import: "default",
});
const bodies = import.meta.glob<string>("../content/tutorials/*/lesson.md", {
  eager: true,
  query: "?raw",
  import: "default",
});
const scenarios = import.meta.glob<LessonScenario>(
  "../content/tutorials/*/scenario.json",
  { eager: true, import: "default" },
);

function slugOf(path: string): string {
  return path.split("/").slice(-2, -1)[0];
}

export const LESSONS: Lesson[] = Object.entries(metas)
  .map(([path, meta]) => {
    const slug = slugOf(path);
    const order = parseInt(slug.split("-")[0], 10) || 0;
    const bodyKey = Object.keys(bodies).find((p) => slugOf(p) === slug);
    const scenarioKey = Object.keys(scenarios).find((p) => slugOf(p) === slug);
    return {
      slug,
      order,
      title: meta.title,
      summary: meta.summary,
      track: meta.track,
      body: bodyKey ? bodies[bodyKey] : "",
      scenario: scenarioKey
        ? scenarios[scenarioKey]
        : { policy: "{}", snapshot: { users: [], devices: [] } },
    };
  })
  .sort((a, b) => a.order - b.order);

export function lessonBySlug(slug: string | null): Lesson | undefined {
  return LESSONS.find((l) => l.slug === slug);
}

export function adjacentLessons(slug: string): {
  prev?: Lesson;
  next?: Lesson;
} {
  const i = LESSONS.findIndex((l) => l.slug === slug);
  return {
    prev: i > 0 ? LESSONS[i - 1] : undefined,
    next: i >= 0 && i < LESSONS.length - 1 ? LESSONS[i + 1] : undefined,
  };
}

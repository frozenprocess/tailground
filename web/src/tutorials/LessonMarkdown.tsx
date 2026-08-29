import { memo } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import { runShow } from "./show";

// Lesson screenshots live next to the lesson (`img/<name>.png`) and are written
// in markdown as `![caption](name.png)`. Vite hashes them and respects `base`.
const IMAGES = import.meta.glob<string>("../content/tutorials/*/img/*", {
  eager: true,
  query: "?url",
  import: "default",
});
const imageUrl = (name: string) =>
  Object.entries(IMAGES).find(([p]) => p.endsWith(`/img/${name}`))?.[1] ?? name;

const CALLOUTS: Record<string, { label: string; cls: string }> = {
  why: { label: "Why", cls: "why" },
  mistake: { label: "Common mistake", cls: "mistake" },
  tip: { label: "Tip", cls: "tip" },
  note: { label: "Note", cls: "note" },
};

// A callout is a run of blockquote lines whose first line is `> [!why] …`.
// Following `>` lines (with or without the leading marker) are folded in.
const CALLOUT_RE = /(?:^> \[!(why|mistake|tip|note)\].*(?:\n> .*)*)/gim;

interface Segment {
  kind: "md" | "callout";
  text: string;
  callout?: string;
}

function segment(body: string): Segment[] {
  const out: Segment[] = [];
  let last = 0;
  for (const m of body.matchAll(CALLOUT_RE)) {
    const start = m.index ?? 0;
    if (start > last) out.push({ kind: "md", text: body.slice(last, start) });
    const lines = m[0].split("\n").map((l) => l.replace(/^> ?/, ""));
    lines[0] = lines[0].replace(/^\[![a-z]+\]\s?/i, "");
    out.push({
      kind: "callout",
      callout: m[1].toLowerCase(),
      text: lines.join("\n").trim(),
    });
    last = start + m[0].length;
  }
  if (last < body.length) out.push({ kind: "md", text: body.slice(last) });
  return out;
}

function MD({ text }: { text: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      urlTransform={(url) => (url.startsWith("show:") ? url : defaultUrlTransform(url))}
      components={{
        img: ({ src, alt }) => {
          const url = imageUrl(src ?? "");
          return (
            <span className="lesson-fig">
              <a href={url} target="_blank" rel="noreferrer" title="Open full size">
                <img src={url} alt={alt ?? ""} loading="lazy" />
              </a>
              {alt && <span className="lesson-figcap">{alt}</span>}
            </span>
          );
        },
        a: ({ children, href }) =>
          href?.startsWith("show:") ? (
            <button
              type="button"
              className="btn show-me"
              onClick={() => void runShow(href.slice(5))}
            >
              <span aria-hidden="true">▶</span> {children}
            </button>
          ) : (
            <a href={href} target="_blank" rel="noreferrer">
              {children}
            </a>
          ),
      }}
    >
      {text}
    </ReactMarkdown>
  );
}

export const LessonMarkdown = memo(function LessonMarkdown({ body }: { body: string }) {
  return (
    <div className="lesson-md">
      {segment(body).map((s, i) =>
        s.kind === "md" ? (
          <MD key={i} text={s.text} />
        ) : (
          <div key={i} className={`callout callout-${CALLOUTS[s.callout!].cls}`}>
            <div className="callout-label">{CALLOUTS[s.callout!].label}</div>
            <MD text={s.text} />
          </div>
        ),
      )}
    </div>
  );
});

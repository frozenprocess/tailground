import { useEffect, useState } from "react";

const REPO_URL = "https://github.com/frozenprocess/tailground";
const SWAP_MS = 3000;

/** Top-bar link to the project's GitHub repository; opens in a new tab. */
export function GitHubButton() {
  const [star, setStar] = useState(false);
  const [paused, setPaused] = useState(false);

  // Alternate the icon on a timer; hovering or focusing the button holds it.
  // State-driven rather than a CSS animation so it still swaps (instantly, via
  // the global reduced-motion transition rule) when the user prefers less motion.
  useEffect(() => {
    if (paused) return;
    const id = window.setInterval(() => setStar((v) => !v), SWAP_MS);
    return () => window.clearInterval(id);
  }, [paused]);

  return (
    <a
      className={`btn github-btn${star ? " show-star" : ""}`}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      href={REPO_URL}
      target="_blank"
      rel="noopener noreferrer"
      title="Tailground on GitHub — star it if it helped"
      aria-label="Tailground on GitHub (opens in a new tab)"
    >
      {/* The two marks are stacked; .show-star cross-fades between them, nudging
          people to star the repo. */}
      <span className="github-btn-icons" aria-hidden="true">
        <svg className="github-mark" viewBox="0 0 16 16" width="14" height="14" fill="currentColor">
          <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 4 0c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z" />
        </svg>
        <svg className="github-star" viewBox="0 0 16 16" width="14" height="14" fill="currentColor">
          <path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.75.75 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z" />
        </svg>
      </span>
      <span className="github-btn-label">GitHub</span>
    </a>
  );
}

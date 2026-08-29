import { useStore } from "../state/store";
import { useMediaQuery, MOBILE_QUERY } from "../app/useMediaQuery";

/**
 * A "Show me" step can bring a pane forward that hides the lesson (the Tailnet
 * tab on desktop; any pane on mobile). This chip offers the way back.
 */
export function TourNote() {
  const show = useStore((s) => s.tourNote);
  const set = useStore((s) => s.setTourNote);
  const leftTab = useStore((s) => s.leftTab);
  const pane = useStore((s) => s.mobilePane);
  const setPane = useStore((s) => s.setMobilePane);
  const isMobile = useMediaQuery(MOBILE_QUERY);

  const lessonHidden = isMobile ? pane !== "learn" : leftTab !== "learn";
  if (!show || !lessonHidden) return null;
  return (
    <div className="tour-note" role="status">
      <span>Showing you the real panel.</span>
      <button
        type="button"
        className="btn btn-primary"
        onClick={() => {
          setPane("learn");
          set(false);
        }}
      >
        ← Back to the lesson
      </button>
      <button type="button" className="row-del" aria-label="Dismiss" onClick={() => set(false)}>
        ✕
      </button>
    </div>
  );
}

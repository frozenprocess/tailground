import { createPortal } from "react-dom";
import { useEffect, useRef } from "react";
import { canonicalView, useStore } from "../state/store";
import { AboutEngine } from "../about/AboutEngine";
import { toHash } from "./useHashRoute";

/** Top-bar entry to "About the engine"; opens it as a modal over the workspace. */
export function AboutButton() {
  const open = useStore((s) => s.aboutOpen);
  const setOpen = useStore((s) => s.setAboutOpen);
  return (
    <>
      <button
        type="button"
        className={`btn about-btn${open ? " active" : ""}`}
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
        title="What the engine is, what Tailground handles itself, and what isn't simulated"
      >
        about
      </button>
      {open && <AboutModal />}
    </>
  );
}

function AboutModal() {
  const setOpen = useStore((s) => s.setAboutOpen);
  const closeRef = useRef<HTMLButtonElement>(null);

  const close = () => {
    setOpen(false);
    // A `#/about` deep link: point the URL back at the view underneath.
    if (window.location.hash === "#/about") {
      const { leftTab, view, activeLesson } = useStore.getState();
      window.history.replaceState(
        null,
        "",
        window.location.pathname + toHash(canonicalView({ leftTab, view }), activeLesson),
      );
    }
  };

  useEffect(() => {
    closeRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return createPortal(
    <div
      className="consent-backdrop"
      role="presentation"
      onMouseDown={(e) => e.target === e.currentTarget && close()}
    >
      <div
        className="consent-card about-modal"
        role="dialog"
        aria-modal="true"
        aria-label="About the engine"
      >
        <button
          ref={closeRef}
          type="button"
          className="btn about-modal-close"
          aria-label="Close"
          onClick={close}
        >
          ✕
        </button>
        <AboutEngine />
      </div>
    </div>,
    document.body,
  );
}

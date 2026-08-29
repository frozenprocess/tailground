import { useStore, type ThemePref } from "../state/store";

const NEXT: Record<ThemePref, ThemePref> = {
  light: "dark",
  dark: "system",
  system: "light",
};
const LABEL: Record<ThemePref, string> = {
  light: "Light",
  dark: "Dark",
  system: "Auto",
};
const GLYPH: Record<ThemePref, string> = {
  light: "☀",
  dark: "☾",
  system: "◐",
};

/** Topbar control that cycles light → dark → system (follows the OS). */
export function ThemeToggle() {
  const pref = useStore((s) => s.themePref);
  const setThemePref = useStore((s) => s.setThemePref);
  return (
    <button
      className="btn theme-toggle"
      onClick={() => setThemePref(NEXT[pref])}
      title={`Theme: ${LABEL[pref]} — click for ${LABEL[NEXT[pref]]}`}
      aria-label={`Theme: ${LABEL[pref]}. Click to change.`}
    >
      <span aria-hidden="true">{GLYPH[pref]}</span>
      {LABEL[pref]}
    </button>
  );
}

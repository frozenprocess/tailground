import { lazy, Suspense } from "react";
import { useEvaluation } from "./app/useEvaluation";
import { useStore } from "./state/store";
import { MatrixView } from "./matrix/MatrixView";
import { Workspace } from "./app/Workspace";
import { LeftPanel } from "./app/LeftPanel";
import { EditorPane } from "./editor/EditorPane";
import { MobileNav } from "./app/MobileNav";
import { useMediaQuery, MOBILE_QUERY } from "./app/useMediaQuery";
import { useHashRoute } from "./app/useHashRoute";
import { useShareHydration } from "./app/useShareHydration";
import { TourNote } from "./tutorials/TourNote";
import { ShareBanner, ShareButton, ShareConsent } from "./app/Share";
import { ThemeToggle } from "./app/ThemeToggle";
import { AboutButton } from "./app/AboutButton";
import { useThemeSync } from "./app/useTheme";
import { Panel } from "./ui/Panel";
import { ImpactTabs } from "./impact/ImpactTabs";
import { SCENARIO_PRESETS } from "./state/presets";
import "./styles/app.css";

// React Flow + elkjs are ~1.6 MB; only pulled in when the Graph tab is opened.
const GraphView = lazy(() =>
  import("./graph/GraphView").then((m) => ({ default: m.GraphView })),
);

export function App() {
  useShareHydration();
  useHashRoute();
  useThemeSync();
  useEvaluation();

  const engineStatus = useStore((s) => s.engineStatus);
  const evalMs = useStore((s) => s.evalMs);
  const view = useStore((s) => s.view);
  const mobilePane = useStore((s) => s.mobilePane);
  const isMobile = useMediaQuery(MOBILE_QUERY);
  const applyRoute = useStore((s) => s.applyRoute);
  const loadScenario = useStore((s) => s.loadScenario);

  return (
    <div className="app">
      <div className="topbar">
        <span className="brand">
          tailground<span className="dot">.</span>
        </span>
        <select
          className="scenario-select"
          value=""
          onChange={(e) => {
            const sc = SCENARIO_PRESETS.find((x) => x.id === e.target.value);
            if (sc) {
              loadScenario(sc.policy, sc.snapshot);
              applyRoute("matrix", null);
            }
            e.currentTarget.value = "";
          }}
          title="Load a worked example — policy and tailnet together"
        >
          <option value="" disabled>
            Load scenario…
          </option>
          {SCENARIO_PRESETS.map((sc) => (
            <option key={sc.id} value={sc.id} title={sc.blurb}>
              {sc.label}
            </option>
          ))}
        </select>
        <span className="spacer" />
        <ThemeToggle />
        <AboutButton />
        <ShareButton />
        <span className="status">
          {evalMs > 0 && (
            <span style={{ color: "var(--text-subtle)" }}>{evalMs.toFixed(0)} ms</span>
          )}
          <span className={`led ${engineStatus}`} />
          <span className="status-text">
            {engineStatus === "ready"
              ? "engine ready"
              : engineStatus === "error"
                ? "engine failed to load"
                : "loading engine…"}
          </span>
        </span>
      </div>

      <ShareBanner />
      <ShareConsent />
      <TourNote />

      {(() => {
        const left = <LeftPanel />;
        const editor = <EditorPane />;
        const impact =
          view === "graph" ? (
            <Suspense
              fallback={
                <Panel id="impact" title="Connectivity graph" tabs={<ImpactTabs />}>
                  <p className="empty">Loading the graph engine…</p>
                </Panel>
              }
            >
              <GraphView />
            </Suspense>
          ) : (
            <MatrixView />
          );

        if (isMobile) {
          const pane =
            mobilePane === "policy"
              ? editor
              : mobilePane === "matrix" || mobilePane === "graph"
                ? impact
                : left;
          return (
            <>
              <div className="mobile-workspace">{pane}</div>
              <MobileNav />
            </>
          );
        }

        return <Workspace left={left} editor={editor} impact={impact} />;
      })()}
    </div>
  );
}

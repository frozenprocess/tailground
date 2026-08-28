import { useState, type ReactNode } from "react";

function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(" ");
}

interface PanelProps {
  /** Shown in the title bar. Rendered uppercase by CSS. */
  title: ReactNode;
  /** Right-aligned in the title bar, before the window controls (counts, status). */
  badge?: ReactNode;
  /** Tab strip in the bar (a lens switch for this region), right-aligned before the badge. */
  tabs?: ReactNode;
  /** A toolbar row rendered below the bar and above the body. */
  toolbar?: ReactNode;
  /** Adds a collapse affordance; the whole bar toggles it. */
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  /** Controlled collapse — pass with `onCollapsedChange` to lift state (L-D). */
  collapsed?: boolean;
  onCollapsedChange?: (collapsed: boolean) => void;
  /** When provided, a ⤢ button appears; the parent owns the behaviour (L-D). */
  onToggleMaximize?: () => void;
  maximized?: boolean;
  /** Stable id for layout persistence (L-D) — emitted as `data-panel`. */
  id?: string;
  className?: string;
  bodyClassName?: string;
  children: ReactNode;
}

/**
 * The one panel shell. A titled, optionally collapsible / maximizable box:
 *   .pane
 *     .pane-bar      — uppercase title + badge + window controls
 *     .pane-head     — optional local toolbar (`toolbar` prop)
 *     .pane-content  — the body (hidden, not unmounted, while collapsed)
 *
 * L-A introduces the shell and the visual language; wiring the ⤢ control and
 * persisting collapse/size is L-D.
 */
export function Panel({
  title,
  badge,
  tabs,
  toolbar,
  collapsible = false,
  defaultCollapsed = false,
  collapsed: collapsedProp,
  onCollapsedChange,
  onToggleMaximize,
  maximized = false,
  id,
  className,
  bodyClassName,
  children,
}: PanelProps) {
  const [collapsedState, setCollapsedState] = useState(defaultCollapsed);
  const collapsed = collapsible && (collapsedProp ?? collapsedState);

  const toggleCollapsed = () => {
    if (!collapsible) return;
    const next = !collapsed;
    onCollapsedChange?.(next);
    if (collapsedProp === undefined) setCollapsedState(next);
  };

  return (
    <section
      className={cx(
        "pane",
        maximized && "pane-maximized",
        collapsed && "pane-collapsed",
        className,
      )}
      data-panel={id}
    >
      <div
        className={cx("pane-bar", collapsible && "pane-bar-toggle")}
        onClick={collapsible ? toggleCollapsed : undefined}
        role={collapsible ? "button" : undefined}
        tabIndex={collapsible ? 0 : undefined}
        aria-expanded={collapsible ? !collapsed : undefined}
        onKeyDown={
          collapsible
            ? (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  toggleCollapsed();
                }
              }
            : undefined
        }
      >
        {collapsible && (
          <span className="pane-caret" aria-hidden="true">
            {collapsed ? "▸" : "▾"}
          </span>
        )}
        <span className="pane-title">{title}</span>
        {tabs != null && tabs}
        {badge != null && <span className="pane-badge">{badge}</span>}
        {onToggleMaximize && (
          <span className="pane-ctl">
            <button
              type="button"
              className="pane-ctl-btn"
              title={maximized ? "Restore" : "Maximize"}
              aria-pressed={maximized}
              onClick={(e) => {
                e.stopPropagation();
                onToggleMaximize();
              }}
            >
              {maximized ? "⤡" : "⤢"}
            </button>
          </span>
        )}
      </div>

      {toolbar && !collapsed && toolbar}

      <div className={cx("pane-content", bodyClassName)} hidden={collapsed}>
        {children}
      </div>
    </section>
  );
}

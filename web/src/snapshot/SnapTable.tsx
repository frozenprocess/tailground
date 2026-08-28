import type { ReactNode } from "react";

/** A column header: its label, or [label, tooltip]. */
type Head = string | [label: string, title: string];

/** A snapshot section's table plus its "+ add" button. */
export function SnapTable({
  heads,
  addLabel,
  onAdd,
  children,
}: {
  heads: Head[];
  addLabel: string;
  onAdd: () => void;
  children: ReactNode;
}) {
  return (
    <div className="snap-sec">
      <table>
        <thead>
          <tr>
            {heads.map((h) =>
              typeof h === "string" ? (
                <th key={h}>{h}</th>
              ) : (
                <th key={h[0]} title={h[1]}>
                  {h[0]}
                </th>
              ),
            )}
            <th />
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
      <button className="add-row" onClick={onAdd}>
        {addLabel}
      </button>
    </div>
  );
}

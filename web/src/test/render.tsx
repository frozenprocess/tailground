import { act, StrictMode, type ReactNode } from "react";
import { createRoot } from "react-dom/client";

/** Minimal React DOM harness: exercises effects, including StrictMode replay. */
export async function render(children: ReactNode) {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  await act(async () => root.render(<StrictMode>{children}</StrictMode>));
  return async () => {
    await act(async () => root.unmount());
    container.remove();
  };
}

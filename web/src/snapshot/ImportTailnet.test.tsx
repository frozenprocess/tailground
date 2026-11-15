// @vitest-environment jsdom
import { act } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { parse as parseJsonc } from "jsonc-parser";
import { render } from "../test/render";
import { useStore } from "../state/store";
import { ImportTailnetButton } from "./ImportTailnet";

const apiOutput = JSON.stringify({
  devices: [
    { hostname: "laptop", user: "zed@example.com", addresses: ["100.64.0.1"], os: "macOS" },
    { hostname: "srv", tags: ["tag:srv"], addresses: ["100.64.0.2"], os: "linux" },
  ],
});

let unmount: (() => Promise<void>) | undefined;
beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true);
  vi.stubGlobal("fetch", vi.fn());
});
afterEach(async () => {
  await unmount?.();
  unmount = undefined;
  vi.unstubAllGlobals();
  document.body.innerHTML = "";
});

const click = (el: Element | null) => act(async () => void (el as HTMLElement).click());
const byText = (sel: string, text: string) =>
  [...document.querySelectorAll(sel)].find((e) => e.textContent?.trim() === text) ?? null;
async function paste(text: string) {
  const ta = document.querySelector<HTMLTextAreaElement>("#import-paste")!;
  await act(async () => {
    // React tracks the value setter, so go through the native one.
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(ta, text);
    ta.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

it("imports pasted API output: preview, apply, no network", async () => {
  unmount = await render(<ImportTailnetButton />);
  expect(document.querySelector('[role="dialog"]')).toBeNull();

  await click(document.querySelector(".import-btn"));
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  const apply = byText("button", "Replace devices") as HTMLButtonElement;
  expect(apply.disabled).toBe(true);

  await paste(apiOutput);
  expect(document.querySelector(".import-preview")?.textContent).toContain("2 devices");
  expect(document.querySelector(".import-preview")?.textContent).toContain("laptop, srv");
  expect(apply.disabled).toBe(false);

  await click(apply);
  const snap = useStore.getState().snapshot;
  expect(snap.devices.map((d) => d.name)).toEqual(["laptop", "srv"]);
  expect(snap.devices[0]).toMatchObject({ user: "zed@example.com", ipv4: "100.64.0.1", os: "macos" });
  expect(snap.devices[1]).toMatchObject({ tags: ["tag:srv"] });
  expect(snap.users.map((u) => u.email)).toContain("zed@example.com");
  // tag:srv isn't in the default policy's tagOwners, so it's added (owner: the first owner seen).
  expect(JSON.stringify(parseJsonc(useStore.getState().policy).tagOwners["tag:srv"])).toBe('["zed@example.com"]');
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
});

it("shows an error and keeps Replace disabled for bad input; changes nothing", async () => {
  const before = useStore.getState().snapshot;
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  await paste('{"access_token":"tskey-x"}');
  expect(document.querySelector(".import-error")?.textContent).toMatch(/token response/);
  expect((byText("button", "Replace devices") as HTMLButtonElement).disabled).toBe(true);
  await click(byText("button", "Cancel"));
  expect(useStore.getState().snapshot).toBe(before);
});

it("closes on Escape without applying, and switches to the CLI command", async () => {
  const before = useStore.getState().snapshot;
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  expect(document.querySelector(".import-cmd pre")?.textContent).toContain("api.tailscale.com");
  await click(byText("button", "tailscale CLI"));
  expect(document.querySelector(".import-cmd pre")?.textContent).toBe("tailscale status --json");
  await act(async () => void window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" })));
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(useStore.getState().snapshot).toBe(before);
});

it("leaves the policy alone when the tagOwners box is unchecked", async () => {
  const policyBefore = useStore.getState().policy;
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  await paste(apiOutput);
  expect(document.querySelector(".import-tags")?.textContent).toContain("tag:srv");
  await click(document.querySelector<HTMLInputElement>(".import-tags input"));
  await click(byText("button", "Replace devices"));
  expect(useStore.getState().policy).toBe(policyBefore);
  expect(useStore.getState().snapshot.devices.map((d) => d.name)).toEqual(["laptop", "srv"]);
});

it("doesn't offer tagOwners when every tag is already defined", async () => {
  useStore.setState({ policy: '{ "tagOwners": { "tag:srv": ["group:eng"] } }' });
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  await paste(apiOutput);
  expect(document.querySelector(".import-tags")).toBeNull();
});

it("previews and merges services from CLI output", async () => {
  useStore.setState({
    snapshot: { ...useStore.getState().snapshot, services: [{ name: "svc:rps-db", vip: "100.1.1.1", backends: [1] }] },
  });
  const cli = JSON.stringify({
    Self: {
      HostName: "me", DNSName: "me.n.ts.net.", UserID: 1, TailscaleIPs: ["100.64.0.1"],
      CapMap: {
        "services/rps-db": [{ Name: "svc:rps-db", Addrs: ["100.70.222.35"] }],
        "services/cache": [{ Name: "svc:cache", Addrs: ["100.70.222.36"] }],
      },
    },
    Peer: {},
    User: { "1": { LoginName: "me@example.com" } },
  });
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  await paste(cli);
  const preview = document.querySelector(".import-preview")?.textContent ?? "";
  expect(preview).toContain("Merges 2 services by name");
  expect(preview).toContain("1 new, 1 updating");
  await click(byText("button", "Replace devices"));
  expect(useStore.getState().snapshot.services).toEqual([
    { name: "svc:rps-db", vip: "100.70.222.35" },
    { name: "svc:cache", vip: "100.70.222.36" },
  ]);
});

it("previews the owner role and applies it", async () => {
  const cli = JSON.stringify({
    Version: "1.104.5-tabc",
    Self: {
      HostName: "me", DNSName: "me.n.ts.net.", UserID: 1, TailscaleIPs: ["100.64.0.1"],
      CapMap: { "https://tailscale.com/cap/is-owner": null },
    },
    Peer: {},
    User: { "1": { LoginName: "boss@example.com" } },
  });
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  await paste(cli);
  expect(document.querySelector(".import-preview")?.textContent).toContain("boss@example.com (owner)");
  await click(byText("button", "Replace devices"));
  const snap = useStore.getState().snapshot;
  expect(snap.users.find((u) => u.email === "boss@example.com")?.role).toBe("owner");
  expect(snap.devices[0]!.posture).toEqual({ "node:tsVersion": "1.104.5" });
});

it("the API command saves one file with devices, users and services, and a chosen file is read locally", async () => {
  unmount = await render(<ImportTailnetButton />);
  await click(document.querySelector(".import-btn"));
  expect(document.querySelector(".import-cmd pre")?.textContent).toContain("> tailnet.json");
  const cmd = document.querySelector(".import-cmd pre")?.textContent ?? "";
  expect(cmd).toContain("get users");
  expect(cmd).toContain("get services");
  expect(document.querySelector(".import-label")?.textContent).toContain("tailnet.json");

  const input = document.querySelector<HTMLInputElement>(".import-file input")!;
  const file = new File([apiOutput], "tailnet.json", { type: "application/json" });
  // jsdom's File has no text(); browsers do.
  Object.defineProperty(file, "text", { value: async () => apiOutput });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(document.querySelector<HTMLTextAreaElement>("#import-paste")!.value).toBe(apiOutput);
  expect(document.querySelector(".import-preview")?.textContent).toContain("2 devices");
  expect(fetch).not.toHaveBeenCalled();
});

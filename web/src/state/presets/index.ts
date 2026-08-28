// Seed content: policy templates for the editor, mock-tailnet presets for the
// snapshot panel, and matched policy + snapshot scenarios. All hand-authored.

import { POLICY_TEMPLATES } from "./templates";
import { SNAPSHOT_PRESETS } from "./snapshots";

export * from "./templates";
export * from "./snapshots";
export * from "./scenarios";

export const DEFAULT_TEMPLATE = POLICY_TEMPLATES[1]; // default-deny
export const DEFAULT_PRESET = SNAPSHOT_PRESETS[0]; // small-startup

import { describe, expect, it } from "vitest";
import { pairEnds, pairKey, splitSSHKey, sshKey } from "./keys";

describe("engine result keys", () => {
  it("match the engine's formats", () => {
    expect(pairKey(1, 2)).toBe("1->2");
    expect(pairKey(3, "svc:web")).toBe("3->svc:web");
    expect(sshKey("root", 1, 2)).toBe("root@1->2");
  });

  it("split back into their parts", () => {
    expect(pairEnds("ext:corp/relay->5")).toEqual(["ext:corp/relay", "5"]);
    expect(splitSSHKey("root@1->2")).toEqual(["root", "1->2"]);
    expect(pairEnds(splitSSHKey(sshKey("u", 7, 8))[1])).toEqual(["7", "8"]);
  });
});

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import ts from "typescript";
import { expect, it } from "vitest";
import { pairKey, sshKey } from "./keys";

const fixturePath = resolve("../engine/eval/testdata/wire.json");
it("typechecks the shared wire fixture against the public TypeScript interfaces", () => {
  // Compile JSON as a literal rather than casting JSON.parse: literal unions,
  // excess fields, required fields and nested shapes must actually typecheck.
  const file = resolve("src/engine/wire-fixture.ts");
  const source = `import type { EngineRequest, EngineResult } from './types';
    const fixture = ${readFileSync(fixturePath, "utf8").trim()} satisfies {
      request: EngineRequest; result: EngineResult;
      minimalRequest: EngineRequest; minimalResult: EngineResult;
    }; export default fixture;`;
  const options: ts.CompilerOptions = {
    strict: true, noEmit: true, skipLibCheck: true,
    target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, moduleResolution: ts.ModuleResolutionKind.Bundler, types: []
  };
  const host = ts.createCompilerHost(options);
  const original = host.getSourceFile;
  host.getSourceFile = (name, language, onError, createNew) => name === file
    ? ts.createSourceFile(file, source, ts.ScriptTarget.ES2022, true)
    : original(name, language, onError, createNew);
  const program = ts.createProgram([file], options, host);
  const errors = ts.getPreEmitDiagnostics(program).map(d => ts.flattenDiagnosticMessageText(d.messageText, "\n"));
  expect(errors).toEqual([]);
});
it("uses the same composite keys and preserves numeric-ID object keys", () => {
  const { result } = JSON.parse(readFileSync(fixturePath, "utf8"));
  expect(result.matrices.https[pairKey(1, 2)]).toBe("allow");
  expect(result.matrices.https[pairKey(1, "route:10.0.0.0/24@1")]).toBe("allow");
  expect(result.matrices.https[pairKey(1, "svc:web")]).toBe("allow");
  expect(result.matrices.https[pairKey("ext:corp/relay", 1)]).toBe("deny");
  expect(result.ssh[sshKey("root", 1, 2)]).toBe("accept");
  expect(result.nodeCaps["1"]["example.com/cap"]).toEqual(["{}"]);
});

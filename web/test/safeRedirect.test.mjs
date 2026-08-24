import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

const source = await readFile(new URL("../src/lib/safeRedirect.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ES2022,
    target: ts.ScriptTarget.ES2022,
  },
});
const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled.outputText).toString("base64")}`;
const { safeRedirectTarget } = await import(moduleURL);

const origin = "https://vocat.example";

test("keeps same-origin application paths", () => {
  assert.equal(safeRedirectTarget("/", origin), "/");
  assert.equal(safeRedirectTarget("/devices/1?tab=network#status", origin), "/devices/1?tab=network#status");
});

test("rejects external and ambiguous login redirects", () => {
  for (const redirect of [
    null,
    "",
    "//evil.example/path",
    "https://evil.example/path",
    "\\\\evil.example\\path",
    "/\\evil.example/path",
    "%2F%2Fevil.example/path",
    "/\nevil.example/path",
  ]) {
    assert.equal(safeRedirectTarget(redirect, origin), "/", String(redirect));
  }
});

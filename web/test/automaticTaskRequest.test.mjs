import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

const source = await readFile(new URL("../src/lib/automaticTaskRequest.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 },
});
const { automaticTaskUpdate } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled.outputText).toString("base64")}`
);

test("automatic task updates omit server-owned fields", () => {
  const task = {
    id: 7,
    name: "test",
    enabled: true,
    deviceId: "device",
    profileIccid: "profile",
    profileAid: "aid",
    taskType: "sms",
    environment: "vowifi",
    intervalDays: 1,
    startDate: "2026-08-26",
    runTime: "20:00",
    timezone: "Asia/Shanghai",
    payload: { phone: "redacted", message: "test" },
    retryCount: 1,
    notify: true,
    nextRunAt: "server-owned",
    lastStatus: "server-owned",
    lastError: "server-owned",
    createdAt: "server-owned",
    updatedAt: "server-owned",
  };

  assert.deepEqual(Object.keys(automaticTaskUpdate(task, false)).sort(), [
    "deviceId", "enabled", "environment", "intervalDays", "name", "notify", "payload",
    "profileAid", "profileIccid", "retryCount", "runTime", "startDate", "taskType", "timezone",
  ]);
  assert.equal(automaticTaskUpdate(task, false).enabled, false);
});

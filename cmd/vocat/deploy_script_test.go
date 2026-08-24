package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const deployCommit = "0123456789abcdef0123456789abcdef01234567"

func TestHardenedDeployScriptSuccess(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, nil)
	if result.Err != nil {
		t.Fatalf("deploy failed: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.releaseDir())
	test.assertState(t, "active")
	if got := test.readDB(t); got != "old database\n" {
		t.Fatalf("database changed during successful preflight deploy: %q", got)
	}
	if !strings.Contains(test.readLog(t), "stop vocat.service") || !strings.Contains(test.readLog(t), "start vocat.service") {
		t.Fatalf("active service was not restarted: %s", test.readLog(t))
	}
	backups, err := os.ReadDir(filepath.Join(test.root, "var", "backups", "vocat"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("validated rollback backup was not retained outside the service data directory: %v, %v", backups, err)
	}
	if _, err := os.Stat(filepath.Join(test.root, "var", "lib", "vocat", "backups")); !os.IsNotExist(err) {
		t.Fatalf("service-writable backup directory exists: %v", err)
	}
}

func TestHardenedDeployScriptBootstrapsNewDatabaseBeforeActivation(t *testing.T) {
	test := newDeployScriptTest(t)
	if err := os.Remove(filepath.Join(test.root, "var", "lib", "vocat", "vocat.db")); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, map[string]string{"VOCAT_DEPLOY_BOOTSTRAP_PASSWORD": "a-long-test-password"})
	if result.Err != nil {
		t.Fatalf("initial deployment failed: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.releaseDir())
	test.assertState(t, "active")
	if got := test.readDB(t); got != "bootstrapped database\n" {
		t.Fatalf("new database was not prepared from the preflight copy: %q", got)
	}
}

func TestHardenedDeployScriptRejectsMigrationBeforeStop(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_MIGRATION_FAIL": "1"})
	if result.Err == nil || !strings.Contains(result.Output, "database preflight failed") {
		t.Fatalf("migration failure was not reported: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
	if strings.Contains(test.readLog(t), "stop vocat.service") {
		t.Fatalf("service stopped before migration preflight: %s", test.readLog(t))
	}
}

func TestHardenedDeployScriptRollsBackReadinessFailure(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_READY": "fail"})
	if result.Err == nil || !strings.Contains(result.Output, "candidate failed readiness") {
		t.Fatalf("readiness failure was not reported: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
	if got := test.readDB(t); got != "old database\n" {
		t.Fatalf("database was not restored: %q", got)
	}
	log := test.readLog(t)
	if strings.Count(log, "start vocat.service") < 2 {
		t.Fatalf("previous service was not restarted after rollback: %s", log)
	}
}

func TestHardenedDeployScriptRejectsUnhealthyRollback(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		want  string
	}{
		{
			name: "readiness",
			extra: map[string]string{
				"VOCAT_TEST_ROLLBACK_READY": "fail",
			},
			want: "previous service failed readiness",
		},
		{
			name: "executable identity",
			extra: map[string]string{
				"VOCAT_TEST_ROLLBACK_EXECUTABLE": "candidate",
			},
			want: "previous service main process is not running the activated release",
		},
		{
			name: "main process",
			extra: map[string]string{
				"VOCAT_TEST_ROLLBACK_MAIN_PID": "0",
			},
			want: "previous service has no live main process after readiness",
		},
		{
			name: "listener ownership",
			extra: map[string]string{
				"VOCAT_TEST_ROLLBACK_LISTENER_PID": "9999",
			},
			want: "port 7575 listener is not owned exclusively by the previous service process",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			test := newDeployScriptTest(t)
			test.withExistingRelease(t)
			testCase.extra["VOCAT_TEST_READY"] = "fail"
			result := test.run(t, testCase.extra)
			if result.Err == nil || !strings.Contains(result.Output, "rollback readiness verification failed: "+testCase.want) {
				t.Fatalf("unhealthy rollback was not reported: %v\n%s", result.Err, result.Output)
			}
			test.assertCurrent(t, test.oldRelease)
			test.assertState(t, "inactive")
		})
	}
}

func TestHardenedDeployScriptForceStopsUnhealthyRollback(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{
		"VOCAT_TEST_READY":              "fail",
		"VOCAT_TEST_ROLLBACK_READY":     "fail",
		"VOCAT_TEST_ROLLBACK_STOP_FAIL": "1",
	})
	if result.Err == nil || !strings.Contains(result.Output, "rollback readiness verification failed") {
		t.Fatalf("unhealthy rollback was not reported: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "inactive")
	log := test.readLog(t)
	if !strings.Contains(log, "kill --kill-who=all --signal=SIGKILL vocat.service\nstop vocat.service") {
		t.Fatalf("failed rollback stop did not complete the SIGKILL and second-stop sequence: %s", log)
	}
}

func TestHardenedDeployScriptReportsUnstoppableRollback(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{
		"VOCAT_TEST_READY":              "fail",
		"VOCAT_TEST_ROLLBACK_READY":     "fail",
		"VOCAT_TEST_ROLLBACK_STOP_FAIL": "1",
		"VOCAT_TEST_ROLLBACK_KILL_FAIL": "1",
	})
	if result.Err == nil || !strings.Contains(result.Output, "unverified previous service could not be stopped; isolate the VM immediately") {
		t.Fatalf("unstoppable rollback was not escalated: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
}

func TestHardenedDeployScriptRollsBackCandidateDatabaseDirectoryReplacement(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{
		"VOCAT_TEST_CANDIDATE_DB_ATTACK": "directory",
		"VOCAT_TEST_READY":               "fail",
	})
	if result.Err == nil || !strings.Contains(result.Output, "candidate failed readiness") {
		t.Fatalf("candidate database replacement did not reach rollback: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
	if got := test.readDB(t); got != "old database\n" {
		t.Fatalf("database directory replacement was not rolled back atomically: %q", got)
	}
	info, err := os.Lstat(filepath.Join(test.root, "var", "lib", "vocat", "vocat.db"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("restored database is not a regular file: %v, %v", info, err)
	}
}

func TestHardenedDeployScriptRejectsStaleReadinessResponder(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_CANDIDATE_STATE": "failed"})
	if result.Err == nil || !strings.Contains(result.Output, "candidate did not remain active") {
		t.Fatalf("stale readiness response was accepted: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
}

func TestHardenedDeployScriptRejectsCandidateWithoutMainProcess(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_MAIN_PID": "0"})
	if result.Err == nil || !strings.Contains(result.Output, "no live main process") {
		t.Fatalf("candidate without a main process was accepted: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
}

func TestHardenedDeployScriptRejectsProcessFromOldRelease(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	test.setProcessExecutable(t, "4242", filepath.Join(test.oldRelease, "vocat"))
	result := test.run(t, map[string]string{"VOCAT_TEST_PRESERVE_PROCESS_EXECUTABLE": "1"})
	if result.Err == nil || !strings.Contains(result.Output, "not running the activated release") {
		t.Fatalf("process from the old release was accepted: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
}

func TestHardenedDeployScriptRejectsListenerOwnedByAnotherProcess(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_SECOND_LISTENER_PID": "9999"})
	if result.Err == nil || !strings.Contains(result.Output, "listener is not owned exclusively") {
		t.Fatalf("listener owned by another process was accepted: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
}

func TestHardenedDeployScriptRejectsPIDChangeAfterReadinessResponse(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	test.setProcessExecutable(t, "4343", test.releaseDir()+"/vocat")
	result := test.run(t, map[string]string{
		"VOCAT_DEPLOY_TEST_READY_ATTEMPTS": "2",
		"VOCAT_TEST_SWITCH_PID_AFTER_CURL": "1",
	})
	if result.Err == nil || !strings.Contains(result.Output, "process changed during readiness request") {
		t.Fatalf("PID change after readiness response was accepted: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
}

func TestHardenedDeployScriptRefusesFailedStop(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_STOP_FAIL": "1"})
	if result.Err == nil || !strings.Contains(result.Output, "failed to stop current service") {
		t.Fatalf("stop failure was not fatal: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
	if !strings.Contains(test.readLog(t), "start vocat.service") {
		t.Fatalf("a failed stop did not restore the previously active service: %s", test.readLog(t))
	}
}

func TestHardenedDeployScriptRestoresServiceWhenStoppedStateCannotBeRead(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_STOP_SHOW_FAIL": "1"})
	if result.Err == nil || !strings.Contains(result.Output, "cannot verify that the current service stopped") {
		t.Fatalf("post-stop state failure was not fatal: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
	if !strings.Contains(test.readLog(t), "start vocat.service") {
		t.Fatalf("old service was not restarted after post-stop state failure: %s", test.readLog(t))
	}
}

func TestHardenedDeployScriptKeepsLiveDatabaseWhenRollbackSnapshotFails(t *testing.T) {
	test := newDeployScriptTest(t)
	test.withExistingRelease(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_ROLLBACK_BACKUP_FAIL": "1"})
	if result.Err == nil || !strings.Contains(result.Output, "rollback database backup failed") {
		t.Fatalf("partial rollback snapshot failure was not reported: %v\n%s", result.Err, result.Output)
	}
	test.assertCurrent(t, test.oldRelease)
	test.assertState(t, "active")
	if got := test.readDB(t); got != "old database\n" {
		t.Fatalf("live database was overwritten by a partial rollback snapshot: %q", got)
	}
	backups, err := os.ReadDir(filepath.Join(test.root, "var", "backups", "vocat"))
	if err != nil || len(backups) != 0 {
		t.Fatalf("partial rollback snapshot was retained: %v, %v", backups, err)
	}
}

func TestHardenedDeployScriptRejectsExtraChecksumEntry(t *testing.T) {
	test := newDeployScriptTest(t)
	data, err := os.ReadFile(filepath.Join(test.artifact, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	extra := strings.Repeat("0", 64) + "  ./ignored\n"
	if err := os.WriteFile(filepath.Join(test.artifact, "SHA256SUMS"), append(data, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "missing or unsafe file") {
		t.Fatalf("extra checksum was accepted: %v\n%s", result.Err, result.Output)
	}
	if _, err := os.Lstat(filepath.Join(test.root, "opt", "vocat", "current")); !os.IsNotExist(err) {
		t.Fatalf("deployment modified current link after checksum rejection: %v", err)
	}
}

func TestHardenedDeployScriptRejectsChecksumPathTraversal(t *testing.T) {
	test := newDeployScriptTest(t)
	data, err := os.ReadFile(filepath.Join(test.artifact, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	extra := strings.Repeat("0", 64) + "  ./reports/../../../etc/shadow\n"
	if err := os.WriteFile(filepath.Join(test.artifact, "SHA256SUMS"), append(data, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "SHA256SUMS") {
		t.Fatalf("checksum path traversal was accepted: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptVerifiesChecksumsBeforeParsingJSON(t *testing.T) {
	test := newDeployScriptTest(t)
	if err := os.WriteFile(filepath.Join(test.artifact, "vocat-linux-amd64"), []byte("tampered\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	jqLog := filepath.Join(test.root, "jq.log")
	result := test.run(t, map[string]string{"VOCAT_TEST_JQ_LOG": jqLog})
	if result.Err == nil || !strings.Contains(result.Output, "artifact checksum verification failed") {
		t.Fatalf("checksum mismatch was accepted: %v\n%s", result.Err, result.Output)
	}
	if data, err := os.ReadFile(jqLog); err == nil {
		t.Fatalf("jq ran before checksum rejection: %q", data)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestHardenedDeployScriptRejectsFailedManifestGate(t *testing.T) {
	test := newDeployScriptTest(t)
	manifestPath := filepath.Join(test.artifact, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"reachable_findings":0`, `"reachable_findings":1`, 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeDeployTestChecksums(t, test.artifact)
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "manifest and build evidence are malformed or inconsistent") {
		t.Fatalf("failed manifest gate was accepted: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptRejectsMalformedOrInconsistentEvidence(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*testing.T, *deployScriptTest)
	}{
		{
			name: "schema type confusion",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					deployTestObject(t, value)["schema"] = "2"
				})
			},
		},
		{
			name: "SBOM component count type confusion",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					manifest := deployTestObject(t, value)
					sbom := deployTestObject(t, manifest["sbom"])
					deployTestObject(t, sbom["source"])["components"] = "1"
				})
			},
		},
		{
			name: "secret report mapping",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					gates := deployTestObject(t, deployTestObject(t, value)["gates"])
					deployTestObject(t, gates["secret_scan"])["report"] = "reports/other.json"
				})
			},
		},
		{
			name: "npm threshold mapping",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					gates := deployTestObject(t, deployTestObject(t, value)["gates"])
					deployTestObject(t, gates["npm_audit_full"])["threshold"] = "critical"
				})
			},
		},
		{
			name: "govuln scan mapping",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					gates := deployTestObject(t, deployTestObject(t, value)["gates"])
					deployTestObject(t, gates["govulncheck_binary"])["scan"] = "package"
				})
			},
		},
		{
			name: "integrity mapping",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					integrity := deployTestObject(t, deployTestObject(t, value)["integrity"])
					integrity["checksums"] = "OTHER"
				})
			},
		},
		{
			name: "toolchain image mapping",
			want: "manifest and build evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
					toolchain := deployTestObject(t, deployTestObject(t, value)["toolchain"])
					deployTestObject(t, toolchain["go"])["image"] = "golang:latest"
				})
			},
		},
		{
			name: "npm install pass marker",
			want: "build evidence does not record a passing gate",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "npm-ci.txt"), "npm output only\nPASS: npm lifecycle rebuild\n", 0o644)
			},
		},
		{
			name: "npm lifecycle pass marker",
			want: "build evidence does not record a passing gate",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "npm-ci.txt"), "PASS: npm ci\nnpm lifecycle output only\n", 0o644)
			},
		},
		{
			name: "full npm audit pass marker",
			want: "build evidence does not record a passing gate",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "npm-audit-full.stderr.txt"), "npm audit output only\n", 0o644)
			},
		},
		{
			name: "production npm audit pass marker",
			want: "build evidence does not record a passing gate",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "npm-audit-production.stderr.txt"), "npm audit output only\n", 0o644)
			},
		},
		{
			name: "Gitleaks finding",
			want: "Gitleaks evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "gitleaks.json"), "[{\"RuleID\":\"test\"}]\n", 0o644)
			},
		},
		{
			name: "multiple Gitleaks documents",
			want: "Gitleaks evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "gitleaks.json"), "[]\n[]\n", 0o644)
			},
		},
		{
			name: "high npm advisory",
			want: "npm audit evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "reports", "npm-audit-full.json"), func(value any) {
					metadata := deployTestObject(t, deployTestObject(t, value)["metadata"])
					vulnerabilities := deployTestObject(t, metadata["vulnerabilities"])
					vulnerabilities["high"] = float64(1)
					vulnerabilities["total"] = float64(1)
				})
			},
		},
		{
			name: "incomplete npm audit",
			want: "npm audit evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				writeDeployTestFile(t, filepath.Join(test.artifact, "reports", "npm-audit-production.json"), "{}\n", 0o644)
			},
		},
		{
			name: "reachable govuln finding",
			want: "govulncheck evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "reports", "govulncheck-source.sarif.json"), func(value any) {
					runs := deployTestObject(t, value)["runs"].([]any)
					deployTestObject(t, runs[0])["results"] = []any{map[string]any{"level": "error"}}
				})
			},
		},
		{
			name: "unknown govuln level",
			want: "govulncheck evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "reports", "govulncheck-binary.sarif.json"), func(value any) {
					runs := deployTestObject(t, value)["runs"].([]any)
					deployTestObject(t, runs[0])["results"] = []any{map[string]any{"level": "none"}}
				})
			},
		},
		{
			name: "empty source SBOM",
			want: "SBOM evidence",
			mutate: func(t *testing.T, test *deployScriptTest) {
				mutateDeployTestJSON(t, filepath.Join(test.artifact, "sbom", "source.cdx.json"), func(value any) {
					deployTestObject(t, value)["components"] = []any{}
				})
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			test := newDeployScriptTest(t)
			testCase.mutate(t, test)
			writeDeployTestChecksums(t, test.artifact)
			result := test.run(t, nil)
			if result.Err == nil || !strings.Contains(result.Output, testCase.want) {
				t.Fatalf("unsafe build evidence was accepted: %v\n%s", result.Err, result.Output)
			}
			if strings.Contains(test.readLog(t), "stop vocat.service") {
				t.Fatalf("service stopped before rejecting build evidence: %s", test.readLog(t))
			}
		})
	}
}

func TestHardenedDeployScriptAcceptsNonBlockingEvidence(t *testing.T) {
	test := newDeployScriptTest(t)
	mutateDeployTestJSON(t, filepath.Join(test.artifact, "reports", "npm-audit-full.json"), func(value any) {
		metadata := deployTestObject(t, deployTestObject(t, value)["metadata"])
		vulnerabilities := deployTestObject(t, metadata["vulnerabilities"])
		vulnerabilities["low"] = float64(1)
		vulnerabilities["moderate"] = float64(1)
		vulnerabilities["total"] = float64(2)
	})
	mutateDeployTestJSON(t, filepath.Join(test.artifact, "reports", "govulncheck-source.sarif.json"), func(value any) {
		runs := deployTestObject(t, value)["runs"].([]any)
		deployTestObject(t, runs[0])["results"] = []any{
			map[string]any{"level": "warning"},
			map[string]any{"level": "note"},
		}
	})
	mutateDeployTestJSON(t, filepath.Join(test.artifact, "manifest.json"), func(value any) {
		gates := deployTestObject(t, deployTestObject(t, value)["gates"])
		deployTestObject(t, gates["govulncheck_source"])["total_findings"] = float64(2)
	})
	writeDeployTestChecksums(t, test.artifact)
	result := test.run(t, nil)
	if result.Err != nil {
		t.Fatalf("non-blocking evidence was rejected: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptRequiresOutOfBandArtifactIdentity(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		test := newDeployScriptTest(t)
		test.expectedCommit = strings.Repeat("a", 40)
		result := test.run(t, nil)
		if result.Err == nil || !strings.Contains(result.Output, "does not match the reviewed commit") {
			t.Fatalf("unexpected manifest commit was accepted: %v\n%s", result.Err, result.Output)
		}
	})

	t.Run("artifact index", func(t *testing.T) {
		test := newDeployScriptTest(t)
		test.expectedIndexSHA256 = test.indexSHA256(t)
		binaryPath := filepath.Join(test.artifact, "vocat-linux-amd64")
		if err := os.WriteFile(binaryPath, []byte("tampered candidate\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeDeployTestChecksums(t, test.artifact)
		result := test.run(t, nil)
		if result.Err == nil || !strings.Contains(result.Output, "does not match the reviewed out-of-band value") {
			t.Fatalf("self-consistent but unreviewed artifact was accepted: %v\n%s", result.Err, result.Output)
		}
	})
}

func TestHardenedDeployScriptRequiresFrontendGates(t *testing.T) {
	test := newDeployScriptTest(t)
	manifestPath := filepath.Join(test.artifact, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"npm_test":{"status":"passed"`, `"npm_test":{"status":"failed"`, 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeDeployTestChecksums(t, test.artifact)
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "manifest and build evidence are malformed or inconsistent") {
		t.Fatalf("failed frontend test gate was accepted: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptVerifiesBinaryGoMetadata(t *testing.T) {
	t.Run("manifest mapping", func(t *testing.T) {
		test := newDeployScriptTest(t)
		manifestPath := filepath.Join(test.artifact, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), `"go_version_m":"reports/go-version-m.txt"`, `"go_version_m":"reports/other.txt"`, 1))
		if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
		writeDeployTestChecksums(t, test.artifact)
		result := test.run(t, nil)
		if result.Err == nil || !strings.Contains(result.Output, "Go metadata path is invalid") {
			t.Fatalf("unexpected binary metadata mapping was accepted: %v\n%s", result.Err, result.Output)
		}
	})

	t.Run("toolchain version", func(t *testing.T) {
		test := newDeployScriptTest(t)
		reportPath := filepath.Join(test.artifact, "reports", "go-version-m.txt")
		data, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), "go1.26.7", "go1.26.6", 1))
		if err := os.WriteFile(reportPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
		writeDeployTestChecksums(t, test.artifact)
		result := test.run(t, nil)
		if result.Err == nil || !strings.Contains(result.Output, "binary Go metadata does not prove") {
			t.Fatalf("wrong binary Go version was accepted: %v\n%s", result.Err, result.Output)
		}
	})
}

func TestHardenedDeployScriptRejectsSymlinkedManifest(t *testing.T) {
	test := newDeployScriptTest(t)
	manifest := filepath.Join(test.artifact, "manifest.json")
	external := filepath.Join(test.root, "external-manifest.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeDeployTestFile(t, external, string(data), 0o644)
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, manifest); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "symbolic link or special file") {
		t.Fatalf("symlinked manifest was accepted: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptRejectsSymlinkedPrivateLock(t *testing.T) {
	test := newDeployScriptTest(t)
	lockDir := filepath.Join(test.root, "run", "vocat-deploy")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(test.root, "lock-sentinel")
	writeDeployTestFile(t, sentinel, "unchanged\n", 0o600)
	if err := os.Symlink(sentinel, filepath.Join(lockDir, "deploy.lock")); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "lock file is unsafe") {
		t.Fatalf("symlinked private lock was accepted: %v\n%s", result.Err, result.Output)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "unchanged\n" {
		t.Fatalf("symlink target was modified: %q (%v)", data, err)
	}
}

func TestHardenedDeployScriptValidatesRootOnlyArtifactSnapshot(t *testing.T) {
	test := newDeployScriptTest(t)
	result := test.run(t, map[string]string{"VOCAT_TEST_MUTATE_INPUT_DURING_JQ": "1"})
	if result.Err != nil {
		t.Fatalf("deploy from validated snapshot failed: %v\n%s", result.Err, result.Output)
	}
	releaseBinary, err := os.ReadFile(filepath.Join(test.releaseDir(), "vocat"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(releaseBinary), "tampered-input") {
		t.Fatal("release was copied from the mutable input after validation started")
	}
}

func TestHardenedDeployScriptDoesNotPreserveArtifactModes(t *testing.T) {
	test := newDeployScriptTest(t)
	if err := os.Chmod(test.artifact, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(test.artifact, "reports"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(test.artifact, "vocat-linux-amd64"), 0o4777); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, map[string]string{"VOCAT_TEST_REQUIRE_SNAPSHOT_ROOT_ONLY": "1"})
	if result.Err != nil {
		t.Fatalf("deploy did not normalize hostile artifact modes: %v\n%s", result.Err, result.Output)
	}

	scriptBytes, err := os.ReadFile("../../scripts/deploy-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	if !strings.Contains(script, "cp -R --no-dereference --no-preserve=all") {
		t.Fatal("artifact copy does not explicitly discard modes, ownership, ACLs, and xattrs")
	}
	if strings.Contains(script, "cp -a --") {
		t.Fatal("artifact snapshot still uses archive-mode copying")
	}
}

func TestHardenedDeployScriptRequiresBuildEvidence(t *testing.T) {
	test := newDeployScriptTest(t)
	if err := os.Remove(filepath.Join(test.artifact, "reports", "go-mod-verify.txt")); err != nil {
		t.Fatal(err)
	}
	writeDeployTestChecksums(t, test.artifact)
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "required build evidence") {
		t.Fatalf("missing build evidence was accepted: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptRejectsSymlinkedReleaseDirectory(t *testing.T) {
	test := newDeployScriptTest(t)
	external := filepath.Join(test.root, "external-release")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(test.releaseDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, test.releaseDir()); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "release path must not be a symbolic link") {
		t.Fatalf("symlinked release directory was accepted: %v\n%s", result.Err, result.Output)
	}
}

func TestHardenedDeployScriptRejectsCurrentOutsideReleaseRoot(t *testing.T) {
	test := newDeployScriptTest(t)
	external := filepath.Join(test.root, "external-current")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(test.root, "opt", "vocat", "current")
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, current); err != nil {
		t.Fatal(err)
	}
	result := test.run(t, nil)
	if result.Err == nil || !strings.Contains(result.Output, "current release points outside") {
		t.Fatalf("external current target was accepted: %v\n%s", result.Err, result.Output)
	}
	test.assertState(t, "active")
	if strings.Contains(test.readLog(t), "stop vocat.service") {
		t.Fatalf("service stopped before current target rejection: %s", test.readLog(t))
	}
}

func TestHardenedDeployScriptDoesNotExecuteArtifactBeforePrivilegeDrop(t *testing.T) {
	test := newDeployScriptTest(t)
	marker := filepath.Join(test.root, "direct-exec")
	result := test.run(t, map[string]string{"VOCAT_TEST_DIRECT_EXEC_MARKER": marker})
	if result.Err != nil {
		t.Fatalf("deploy failed: %v\n%s", result.Err, result.Output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("artifact was executed directly by the deploy script: %v", err)
	}
}

func TestHardenedDeployScriptSandboxesCandidatePreflight(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/deploy-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		`readonly PREFLIGHT_USER="vocat-preflight"`,
		`readonly PREFLIGHT_ROOT="/var/lib/vocat-preflight"`,
		`preflight_dir="$(mktemp -d "$PREFLIGHT_ROOT/run.XXXXXX")"`,
		`--property="InaccessiblePaths=$DATA_DIR $BACKUP_DIR $LOCK_DIR"`,
		`--property="TemporaryFileSystem=/run:rw,nosuid,nodev,noexec,mode=0700"`,
		`--property=PrivateNetwork=yes`,
		`--property=PrivateIPC=yes`,
		`--property=PrivateDevices=yes`,
		`--property=ProtectSystem=strict`,
		`--property=CapabilityBoundingSet=`,
		`--property=RuntimeMaxSec=2min`,
		`--property=MemoryMax=512M`,
		`--property=MemorySwapMax=0`,
		`--property=TasksMax=64`,
		`--property=LimitNOFILE=1024`,
		`--property=CPUQuota=200%`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("candidate preflight sandbox is missing %q", required)
		}
	}
	if strings.Contains(script, `runuser -u "$SERVICE_USER" -- "$release_dir/vocat"`) {
		t.Fatal("candidate binary preflight still runs as the live service account")
	}
}

func TestHardenedDeployScriptRejectsUnsafePreflightOutputs(t *testing.T) {
	tests := []struct {
		name   string
		attack string
	}{
		{name: "symbolic link", attack: "symlink"},
		{name: "fifo", attack: "fifo"},
		{name: "extra output", attack: "extra"},
		{name: "writable database", attack: "mode"},
		{name: "hard linked database", attack: "hardlink"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			test := newDeployScriptTest(t)
			test.withExistingRelease(t)
			sentinel := filepath.Join(test.root, "preflight-sentinel")
			writeDeployTestFile(t, sentinel, "must remain unchanged\n", 0o600)
			result := test.run(t, map[string]string{
				"VOCAT_TEST_PREFLIGHT_ATTACK":        testCase.attack,
				"VOCAT_TEST_PREFLIGHT_ATTACK_TARGET": sentinel,
			})
			if result.Err == nil || (!strings.Contains(result.Output, "candidate preflight produced unexpected output") &&
				!strings.Contains(result.Output, "candidate database has unsafe")) {
				t.Fatalf("unsafe preflight output was accepted: %v\n%s", result.Err, result.Output)
			}
			if strings.Contains(test.readLog(t), "stop vocat.service") {
				t.Fatalf("service stopped before preflight output validation: %s", test.readLog(t))
			}
			data, err := os.ReadFile(sentinel)
			if err != nil || string(data) != "must remain unchanged\n" {
				t.Fatalf("preflight attack target changed: %q (%v)", data, err)
			}
		})
	}
}

func TestHardenedDeployScriptRejectsUnsafeLiveDatabaseState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *deployScriptTest)
	}{
		{
			name: "database symlink",
			mutate: func(t *testing.T, test *deployScriptTest) {
				database := filepath.Join(test.root, "var", "lib", "vocat", "vocat.db")
				target := filepath.Join(test.root, "database-symlink-target")
				writeDeployTestFile(t, target, "not a database\n", 0o600)
				if err := os.Remove(database); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, database); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "database hard link",
			mutate: func(t *testing.T, test *deployScriptTest) {
				database := filepath.Join(test.root, "var", "lib", "vocat", "vocat.db")
				if err := os.Link(database, filepath.Join(test.root, "database-hard-link")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "database mode",
			mutate: func(t *testing.T, test *deployScriptTest) {
				if err := os.Chmod(filepath.Join(test.root, "var", "lib", "vocat", "vocat.db"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "WAL symlink",
			mutate: func(t *testing.T, test *deployScriptTest) {
				target := filepath.Join(test.root, "wal-symlink-target")
				writeDeployTestFile(t, target, "untrusted\n", 0o600)
				if err := os.Symlink(target, filepath.Join(test.root, "var", "lib", "vocat", "vocat.db-wal")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "SHM hard link",
			mutate: func(t *testing.T, test *deployScriptTest) {
				shm := filepath.Join(test.root, "var", "lib", "vocat", "vocat.db-shm")
				writeDeployTestFile(t, shm, "untrusted\n", 0o600)
				if err := os.Link(shm, filepath.Join(test.root, "shm-hard-link")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			test := newDeployScriptTest(t)
			test.withExistingRelease(t)
			testCase.mutate(t, test)
			result := test.run(t, nil)
			if result.Err == nil || !strings.Contains(result.Output, "has unsafe type, ownership, mode, or link count") {
				t.Fatalf("unsafe live database state was accepted: %v\n%s", result.Err, result.Output)
			}
			if strings.Contains(test.readLog(t), "stop vocat.service") {
				t.Fatalf("service stopped before rejecting unsafe live database state: %s", test.readLog(t))
			}
		})
	}
}

type deployScriptTest struct {
	root                string
	artifact            string
	oldRelease          string
	binDir              string
	realJQ              string
	expectedCommit      string
	expectedIndexSHA256 string
	procRoot            string
}

func newDeployScriptTest(t *testing.T) *deployScriptTest {
	t.Helper()
	realJQ, err := exec.LookPath("jq")
	if err != nil {
		t.Fatal("deployment tests require jq:", err)
	}
	root := t.TempDir()
	test := &deployScriptTest{
		root:           root,
		artifact:       filepath.Join(root, "artifact"),
		oldRelease:     filepath.Join(root, "opt", "vocat", "releases", "old"),
		binDir:         filepath.Join(root, "bin"),
		realJQ:         realJQ,
		expectedCommit: deployCommit,
		procRoot:       filepath.Join(root, "proc"),
	}
	for _, dir := range []string{test.artifact, test.binDir, test.procRoot, filepath.Join(root, "var", "lib", "vocat")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeDeployTestFile(t, filepath.Join(test.artifact, "vocat-linux-amd64"), `#!/bin/sh
if [ "$1" = version ]; then
  : > "${VOCAT_TEST_DIRECT_EXEC_MARKER:?}"
  exit 0
fi
if [ "$1" = bootstrap-admin ]; then
  if [ "${VOCAT_TEST_MIGRATION_FAIL:-}" = 1 ]; then exit 42; fi
  database=
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --database ]; then database=$2; shift 2; continue; fi
    shift
  done
  [ -n "$database" ] || exit 2
  printf 'bootstrapped database\n' > "$database"
  case "${VOCAT_TEST_PREFLIGHT_ATTACK:-}" in
    symlink)
      rm -f -- "$database"
      ln -s "${VOCAT_TEST_PREFLIGHT_ATTACK_TARGET:?}" "$database"
      ;;
    fifo)
      rm -f -- "$database"
      mkfifo "$database"
      ;;
    extra)
      printf 'unexpected\n' > "$(dirname "$database")/unexpected"
      ;;
    mode)
      chmod 0666 "$database"
      ;;
    hardlink)
      ln "$database" "$(dirname "$database")/unexpected"
      ;;
  esac
  exit 0
fi
exit 2
`, 0o755)
	binary, err := os.ReadFile(filepath.Join(test.artifact, "vocat-linux-amd64"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	hexSum := hex.EncodeToString(sum[:])
	manifest := fmt.Sprintf(`{
  "schema": 2,
  "source": {"type":"git archive","commit":%q,"frontend_dist":{"isolated_build":true,"files":1,"size_bytes":1}},
  "target": "linux/amd64",
  "build_platform": "linux/amd64",
  "version": "hardened-%s",
  "build_time": "2026-08-24T00:00:00Z",
  "binary": {"file":%q,"sha256":%q,"go_version_m":"reports/go-version-m.txt"},
  "toolchain": {
    "go":{"image":"golang:1.26.7-alpine3.23@sha256:b17af760035fc2f338eed92d448a6c67f2d45438844fc6c60678fa5f99e44b57","version":"go version go1.26.7 linux/amd64"},
    "go_test":{"image":"golang:1.26.7-bookworm@sha256:6ef6e30f0ea5c384f6d111cf856e024e3086bbdcb1779da3f3b3fbba0aea53d2","version":"go version go1.26.7 linux/amd64"},
    "go_race":{"image":"golang:1.26.7-bookworm@sha256:6ef6e30f0ea5c384f6d111cf856e024e3086bbdcb1779da3f3b3fbba0aea53d2","version":"go version go1.26.7 linux/amd64"},
    "node":{"image":"node:24.15.0-alpine3.23@sha256:d1b3b4da11eefd5941e7f0b9cf17783fc99d9c6fc34884a665f40a06dbdfc94f","version":"v24.15.0"},
    "npm":{"version":"11.12.1"},
    "jq":{"image":"ghcr.io/jqlang/jq:1.8.1@sha256:4f34c6d23f4b1372ac789752cc955dc67c2ae177eb1b5860b75cdc5091ce6f91","version":"jq-1.8.1"},
    "govulncheck":{"version":"v1.7.0","report":"reports/govulncheck-version.txt"},
    "gitleaks":{"image":"zricethezav/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f","version":"v8.30.1"},
    "syft":{"image":"anchore/syft:v1.51.0@sha256:678bfa565b60f747aac0f8e964fe5588a24445b8d0a480e91f6efd70020dfbb0","version":"1.51.0"}
  },
  "gates": {
    "secret_scan":{"status":"passed","findings":0,"report":"reports/gitleaks.json"},
    "npm_audit_full":{"status":"passed","threshold":"high","high":0,"critical":0,"report":"reports/npm-audit-full.json"},
    "npm_audit_production":{"status":"passed","threshold":"high","high":0,"critical":0,"report":"reports/npm-audit-production.json"},
    "npm_test":{"status":"passed","report":"reports/npm-test.txt"},
    "npm_build":{"status":"passed","report":"reports/npm-build.txt"},
    "go_mod_download":{"status":"passed","report":"reports/go-mod-download.txt"},
    "go_mod_verify":{"status":"passed","report":"reports/go-mod-verify.txt"},
    "go_test":{"status":"passed","report":"reports/go-test.txt"},
    "go_test_race":{"status":"passed","report":"reports/go-test-race.txt"},
    "go_vet":{"status":"passed","report":"reports/go-vet.txt"},
    "govulncheck_source":{"status":"passed","scan":"symbol","reachable_findings":0,"total_findings":0,"report":"reports/govulncheck-source.sarif.json"},
    "govulncheck_binary":{"status":"passed","scan":"symbol","reachable_findings":0,"total_findings":0,"report":"reports/govulncheck-binary.sarif.json"}
  },
  "sbom": {
    "source":{"format":"CycloneDX JSON","components":1,"file":"sbom/source.cdx.json"},
    "binary":{"format":"CycloneDX JSON","components":1,"file":"sbom/binary.cdx.json"}
  },
  "integrity":{"checksums":"SHA256SUMS","scope":"all regular artifact files except SHA256SUMS"}
}`, deployCommit, deployCommit[:12], "vocat-linux-amd64", hexSum) + "\n"
	writeDeployTestFile(t, filepath.Join(test.artifact, "manifest.json"), manifest, 0o644)
	for path, content := range map[string]string{
		"go-version.txt":                          "go version go1.26.7 linux/amd64\n",
		"node-version.txt":                        "v24.15.0\n",
		"npm-version.txt":                         "11.12.1\n",
		"reports/gitleaks-version.txt":            "v8.30.1\n",
		"reports/gitleaks.json":                   "[]\n",
		"reports/go-build.txt":                    "PASS: static Go build\n",
		"reports/go-mod-download.txt":             "PASS: go mod download\n",
		"reports/go-mod-verify.txt":               "PASS: go mod verify\n",
		"reports/go-race-version.txt":             "go version go1.26.7 linux/amd64\n",
		"reports/go-test.txt":                     "PASS: go test\n",
		"reports/go-test-race.txt":                "PASS: go test -race\n",
		"reports/go-test-version.txt":             "go version go1.26.7 linux/amd64\n",
		"reports/go-vet.txt":                      "PASS: go vet\n",
		"reports/go-version-m.txt":                "/artifact/vocat-linux-amd64: go1.26.7\n\tpath\tvocat/cmd/vocat\n\tbuild\tCGO_ENABLED=0\n\tbuild\tGOARCH=amd64\n\tbuild\tGOOS=linux\n",
		"reports/govulncheck-version.txt":         "Scanner: govulncheck@v1.7.0\n",
		"reports/govulncheck-source.sarif.json":   "{\"version\":\"2.1.0\",\"runs\":[{\"tool\":{\"driver\":{\"name\":\"govulncheck\"}},\"results\":[]}]}\n",
		"reports/govulncheck-binary.sarif.json":   "{\"version\":\"2.1.0\",\"runs\":[{\"tool\":{\"driver\":{\"name\":\"govulncheck\"}},\"results\":[]}]}\n",
		"reports/jq-version.txt":                  "jq-1.8.1\n",
		"reports/npm-audit-full.json":             "{\"auditReportVersion\":2,\"metadata\":{\"vulnerabilities\":{\"info\":0,\"low\":0,\"moderate\":0,\"high\":0,\"critical\":0,\"total\":0},\"dependencies\":{\"prod\":1,\"dev\":1,\"optional\":0,\"peer\":0,\"peerOptional\":0,\"total\":2}}}\n",
		"reports/npm-audit-full.stderr.txt":       "PASS: npm audit (all dependencies, high+)\n",
		"reports/npm-audit-production.json":       "{\"auditReportVersion\":2,\"metadata\":{\"vulnerabilities\":{\"info\":0,\"low\":0,\"moderate\":0,\"high\":0,\"critical\":0,\"total\":0},\"dependencies\":{\"prod\":1,\"dev\":0,\"optional\":0,\"peer\":0,\"peerOptional\":0,\"total\":1}}}\n",
		"reports/npm-audit-production.stderr.txt": "PASS: npm audit (production dependencies, high+)\n",
		"reports/npm-build.txt":                   "PASS: npm build\n",
		"reports/npm-ci.txt":                      "PASS: npm ci\nPASS: npm lifecycle rebuild\n",
		"reports/npm-test.txt":                    "PASS: npm test\n",
		"reports/syft-version.json":               "{\"version\":\"1.51.0\"}\n",
		"sbom/source.cdx.json":                    "{\"bomFormat\":\"CycloneDX\",\"components\":[{\"type\":\"application\",\"name\":\"vocat-source\"}]}\n",
		"sbom/binary.cdx.json":                    "{\"bomFormat\":\"CycloneDX\",\"components\":[{\"type\":\"application\",\"name\":\"vocat-binary\"}]}\n",
	} {
		writeDeployTestFile(t, filepath.Join(test.artifact, path), content, 0o644)
	}
	writeDeployTestChecksums(t, test.artifact)

	writeDeployTestFile(t, filepath.Join(test.binDir, "jq"), `#!/bin/sh
if [ -n "${VOCAT_TEST_JQ_LOG:-}" ]; then
  printf 'jq\n' >> "$VOCAT_TEST_JQ_LOG"
fi
if [ "${2:-}" = .schema ]; then
  file=${3:?}
  if [ "${VOCAT_TEST_REQUIRE_SNAPSHOT_ROOT_ONLY:-}" = 1 ]; then
    snapshot=${file%/manifest.json}
    [ "$(stat -c '%a' "$snapshot")" = 700 ] || exit 91
    bad=$(find "$snapshot" \( -type d ! -perm 0700 -o -type f ! -perm 0600 \) -print -quit)
    [ -z "$bad" ] || exit 92
  fi
  if [ "${VOCAT_TEST_MUTATE_INPUT_DURING_JQ:-}" = 1 ] && [ ! -e "${VOCAT_TEST_ARTIFACT:?}/.mutated" ]; then
    printf 'tampered-input\n' > "$VOCAT_TEST_ARTIFACT/vocat-linux-amd64"
    : > "$VOCAT_TEST_ARTIFACT/.mutated"
  fi
fi
exec "${VOCAT_TEST_REAL_JQ:?}" "$@"
`, 0o755)
	writeDeployTestFile(t, filepath.Join(test.binDir, "systemd-run"), `#!/bin/sh
private_run=false
private_network=false
runtime_limit=false
memory_limit=false
tasks_limit=false
fd_limit=false
cpu_limit=false
while [ "$#" -gt 0 ]; do
  if [ "$1" = -- ]; then shift; break; fi
  case "$1" in
    --property=TemporaryFileSystem=/run:rw,nosuid,nodev,noexec,mode=0700) private_run=true;;
    --property=PrivateNetwork=yes) private_network=true;;
    --property=RuntimeMaxSec=2min) runtime_limit=true;;
    --property=MemoryMax=512M) memory_limit=true;;
    --property=TasksMax=64) tasks_limit=true;;
    --property=LimitNOFILE=1024) fd_limit=true;;
    --property=CPUQuota=200%) cpu_limit=true;;
  esac
  shift
done
[ "$private_run" = true ] && [ "$private_network" = true ] && [ "$runtime_limit" = true ] &&
  [ "$memory_limit" = true ] && [ "$tasks_limit" = true ] && [ "$fd_limit" = true ] &&
  [ "$cpu_limit" = true ] || exit 97
exec "$@"
`, 0o755)
	writeDeployTestFile(t, filepath.Join(test.binDir, "runuser"), `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in -u) shift 2;; --) shift; break;; *) shift;; esac
done
exec "$@"
`, 0o755)
	writeDeployTestFile(t, filepath.Join(test.binDir, "sqlite3"), `#!/bin/sh
db=$1
cmd=${2:-}
case "$cmd" in
	  ".backup '"*)
	    out=${cmd#".backup '"}; out=${out%"'"}
	    if [ "${VOCAT_TEST_ROLLBACK_BACKUP_FAIL:-}" = 1 ] && [ -e "${VOCAT_TEST_STATE:?}.snapshot-complete" ]; then
	      printf 'partial\n' > "$out"
	      exit 42
	    fi
	    cp "$db" "$out"
	    : > "${VOCAT_TEST_STATE:?}.snapshot-complete"
    ;;
  'PRAGMA quick_check;') printf 'ok\n';;
  'PRAGMA wal_checkpoint(TRUNCATE);') :;;
esac
`, 0o755)
	writeDeployTestFile(t, filepath.Join(test.binDir, "systemctl"), `#!/bin/sh
log=${VOCAT_TEST_LOG:?}
state=${VOCAT_TEST_STATE:?}
current_release() {
  readlink -f -- "${VOCAT_TEST_CURRENT_LINK:?}"
}
is_candidate() {
  [ "$(current_release)" = "${VOCAT_TEST_CANDIDATE_RELEASE:?}" ]
}
case "$1" in
  cat) :;;
  show)
    case "$2" in
      --property=ActiveState)
        if [ "${VOCAT_TEST_STOP_SHOW_FAIL:-}" = 1 ] && [ -e "$state.stopped" ] && [ ! -e "$state.show-failed" ]; then
          : > "$state.show-failed"
          exit 1
        fi
        cat "$state"; printf '\n'
        ;;
	      --property=MainPID)
	        if [ "$(cat "$state")" = active ]; then
	          if is_candidate && [ -n "${VOCAT_TEST_MAIN_PID:-}" ]; then
	            printf '%s\n' "$VOCAT_TEST_MAIN_PID"
	          elif ! is_candidate && [ -n "${VOCAT_TEST_ROLLBACK_MAIN_PID:-}" ]; then
	            printf '%s\n' "$VOCAT_TEST_ROLLBACK_MAIN_PID"
	          else
	            cat "$state.pid"; printf '\n'
	          fi
	        else
	          printf '0\n'
	        fi
        ;;
      *) exit 2;;
    esac
    ;;
	  stop)
	    echo "stop $2" >> "$log"
	    if ! is_candidate && [ -e "$state.candidate-launched" ] && [ "${VOCAT_TEST_ROLLBACK_STOP_FAIL:-}" = 1 ]; then
	      exit 1
	    fi
	    [ "${VOCAT_TEST_STOP_FAIL:-}" != 1 ] || exit 1
	    printf inactive > "$state"
	    : > "$state.stopped"
	    ;;
	  start)
	    echo "start $2" >> "$log"
	    if is_candidate; then : > "$state.candidate-launched"; fi
    if [ "${VOCAT_TEST_CANDIDATE_DB_ATTACK:-}" = directory ] && [ ! -e "$state.database-attacked" ]; then
      database=${VOCAT_TEST_DATABASE:?}
      rm -f -- "$database"
      mkdir -- "$database"
      printf 'candidate-controlled\n' > "$database/unexpected"
      : > "$state.database-attacked"
    fi
    if [ -n "${VOCAT_TEST_CANDIDATE_STATE:-}" ] && [ ! -e "$state.candidate-started" ]; then
      : > "$state.candidate-started"
      printf '%s' "$VOCAT_TEST_CANDIDATE_STATE" > "$state"
	    else
	      printf active > "$state"
	    fi
	    pid=$(cat "$state.pid")
	    if ! is_candidate || [ "${VOCAT_TEST_PRESERVE_PROCESS_EXECUTABLE:-}" != 1 ]; then
	      executable="$(current_release)/vocat"
	      if ! is_candidate && [ "${VOCAT_TEST_ROLLBACK_EXECUTABLE:-}" = candidate ]; then
	        executable="${VOCAT_TEST_CANDIDATE_RELEASE:?}/vocat"
	      fi
	      mkdir -p -- "${VOCAT_DEPLOY_TEST_PROC_ROOT:?}/$pid"
	      ln -sfn -- "$executable" "${VOCAT_DEPLOY_TEST_PROC_ROOT:?}/$pid/exe"
	    fi
	    ;;
	  kill)
	    echo "kill $*" >> "$log"
	    if ! is_candidate && [ -e "$state.candidate-launched" ] && [ "${VOCAT_TEST_ROLLBACK_KILL_FAIL:-}" = 1 ]; then
	      exit 1
	    fi
	    printf inactive > "$state"
	    ;;
  daemon-reload) echo daemon-reload >> "$log";;
esac
`, 0o755)
	writeDeployTestFile(t, filepath.Join(test.binDir, "curl"), `#!/bin/sh
found_noproxy=false
previous=
for argument in "$@"; do
  if [ "$previous" = --noproxy ] && [ "$argument" = '*' ]; then found_noproxy=true; fi
  previous=$argument
done
[ "$found_noproxy" = true ] || exit 2
current=$(readlink -f -- "${VOCAT_TEST_CURRENT_LINK:?}")
if [ "$current" = "${VOCAT_TEST_CANDIDATE_RELEASE:?}" ]; then
  [ "${VOCAT_TEST_READY:-ok}" = ok ] || exit 1
  if [ "${VOCAT_TEST_SWITCH_PID_AFTER_CURL:-}" = 1 ]; then
    printf '4343' > "${VOCAT_TEST_STATE:?}.pid"
  fi
else
  [ "${VOCAT_TEST_ROLLBACK_READY:-ok}" = ok ] || exit 1
fi
`, 0o755)
	writeDeployTestFile(t, filepath.Join(test.binDir, "ss"), `#!/bin/sh
[ "$*" = "-H -ltnp sport = :7575" ] || exit 2
current=$(readlink -f -- "${VOCAT_TEST_CURRENT_LINK:?}")
if [ "$current" = "${VOCAT_TEST_CANDIDATE_RELEASE:?}" ]; then
  pid=${VOCAT_TEST_LISTENER_PID:-$(cat "${VOCAT_TEST_STATE:?}.pid")}
  second_pid=${VOCAT_TEST_SECOND_LISTENER_PID:-}
else
  pid=${VOCAT_TEST_ROLLBACK_LISTENER_PID:-$(cat "${VOCAT_TEST_STATE:?}.pid")}
  second_pid=${VOCAT_TEST_ROLLBACK_SECOND_LISTENER_PID:-}
fi
printf 'LISTEN 0 4096 0.0.0.0:7575 0.0.0.0:* users:(("vocat",pid=%s,fd=3))\n' "$pid"
if [ -n "$second_pid" ]; then
  printf 'LISTEN 0 4096 [::]:7575 [::]:* users:(("other",pid=%s,fd=4))\n' "$second_pid"
fi
`, 0o755)
	writeDeployTestFile(t, filepath.Join(root, "var", "lib", "vocat", "vocat.db"), "old database\n", 0o600)
	writeDeployTestFile(t, filepath.Join(root, "state"), "active", 0o600)
	writeDeployTestFile(t, filepath.Join(root, "state.pid"), "4242", 0o600)
	writeDeployTestFile(t, filepath.Join(root, "log"), "", 0o600)
	test.setProcessExecutable(t, "4242", test.releaseDir()+"/vocat")
	return test
}

func (d *deployScriptTest) withExistingRelease(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(d.oldRelease, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDeployTestFile(t, filepath.Join(d.oldRelease, "vocat"), "old\n", 0o755)
	current := filepath.Join(d.root, "opt", "vocat", "current")
	if err := os.Symlink(d.oldRelease, current); err != nil {
		t.Fatal(err)
	}
	d.setProcessExecutable(t, "4242", d.oldRelease+"/vocat")
}

func (d *deployScriptTest) releaseDir() string {
	return filepath.Join(d.root, "opt", "vocat", "releases", deployCommit)
}

func (d *deployScriptTest) setProcessExecutable(t *testing.T, pid, executable string) {
	t.Helper()
	processDir := filepath.Join(d.procRoot, pid)
	if err := os.MkdirAll(processDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(processDir, "exe")
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
}

type deployResult struct {
	Output string
	Err    error
}

func (d *deployScriptTest) run(t *testing.T, extra map[string]string) deployResult {
	t.Helper()
	expectedIndexSHA256 := d.expectedIndexSHA256
	if expectedIndexSHA256 == "" {
		expectedIndexSHA256 = d.indexSHA256(t)
	}
	command := exec.Command(
		"bash", "../../scripts/deploy-hardened.sh",
		"--expected-commit", d.expectedCommit,
		"--expected-index-sha256", expectedIndexSHA256,
		d.artifact,
	)
	command.Env = append(os.Environ(),
		"VOCAT_DEPLOY_TEST_ROOT="+d.root,
		"VOCAT_DEPLOY_TEST_USER="+os.Getenv("USER"),
		"VOCAT_DEPLOY_TEST_PREFLIGHT_USER="+os.Getenv("USER"),
		"VOCAT_DEPLOY_TEST_PREFLIGHT_GROUP="+os.Getenv("USER"),
		"VOCAT_DEPLOY_TEST_READY_ATTEMPTS=1",
		"VOCAT_DEPLOY_TEST_PROC_ROOT="+d.procRoot,
		"VOCAT_TEST_ARTIFACT="+d.artifact,
		"VOCAT_TEST_LOG="+filepath.Join(d.root, "log"),
		"VOCAT_TEST_STATE="+filepath.Join(d.root, "state"),
		"VOCAT_TEST_DATABASE="+filepath.Join(d.root, "var", "lib", "vocat", "vocat.db"),
		"VOCAT_TEST_CURRENT_LINK="+filepath.Join(d.root, "opt", "vocat", "current"),
		"VOCAT_TEST_CANDIDATE_RELEASE="+d.releaseDir(),
		"VOCAT_TEST_OLD_RELEASE="+d.oldRelease,
		"VOCAT_TEST_REAL_JQ="+d.realJQ,
		"PATH="+d.binDir+":"+os.Getenv("PATH"),
	)
	for key, value := range extra {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	return deployResult{Output: string(output), Err: err}
}

func (d *deployScriptTest) indexSHA256(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(d.artifact, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (d *deployScriptTest) assertCurrent(t *testing.T, want string) {
	t.Helper()
	got, err := filepath.EvalSymlinks(filepath.Join(d.root, "opt", "vocat", "current"))
	if err != nil || got != want {
		t.Fatalf("current = %q (%v), want %q", got, err, want)
	}
}

func (d *deployScriptTest) assertState(t *testing.T, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(d.root, "state"))
	if err != nil || strings.TrimSpace(string(got)) != want {
		t.Fatalf("service state = %q (%v), want %q", got, err, want)
	}
}

func (d *deployScriptTest) readDB(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(d.root, "var", "lib", "vocat", "vocat.db"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (d *deployScriptTest) readLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(d.root, "log"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mutateDeployTestJSON(t *testing.T, path string, mutate func(any)) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	mutate(value)
	updated, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
}

func deployTestObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("test fixture value is %T, want JSON object", value)
	}
	return object
}

func writeDeployTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func writeDeployTestChecksums(t *testing.T, root string) {
	t.Helper()
	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.Mode().IsRegular() || filepath.Base(path) == "SHA256SUMS" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, hex.EncodeToString(sum[:])+"  ./"+filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	writeDeployTestFile(t, filepath.Join(root, "SHA256SUMS"), strings.Join(entries, "\n")+"\n", 0o644)
}

package main

import (
	"strings"
	"testing"
)

// newTestState returns a zero-value AuditState with output suppressed.
// No terminal output occurs because write() calls fmt.Print — tests must
// tolerate the small amount of terminal noise, or we capture via OutputBuffer.
func newTestState() *AuditState {
	return &AuditState{
		Namespace: "test-ns",
		Domain:    "test.example.com",
	}
}

// ─── counters ────────────────────────────────────────────────────────────────

func TestLogPass_incrementsPassCount(t *testing.T) {
	a := newTestState()
	before := a.PassCount
	a.logPass("everything is fine")
	if a.PassCount != before+1 {
		t.Errorf("PassCount should be %d, got %d", before+1, a.PassCount)
	}
	if a.FailCount != 0 || a.WarnCount != 0 {
		t.Error("logPass must not touch FailCount or WarnCount")
	}
}

func TestLogFail_incrementsFailCount(t *testing.T) {
	a := newTestState()
	a.logFail("something broke")
	if a.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", a.FailCount)
	}
	if a.PassCount != 0 || a.WarnCount != 0 {
		t.Error("logFail must not touch PassCount or WarnCount")
	}
}

func TestLogWarn_incrementsWarnCount(t *testing.T) {
	a := newTestState()
	a.logWarn("slight concern")
	if a.WarnCount != 1 {
		t.Errorf("WarnCount = %d, want 1", a.WarnCount)
	}
}

func TestLogInfo_incrementsInfoCount(t *testing.T) {
	a := newTestState()
	a.logInfo("for your records")
	if a.InfoCount != 1 {
		t.Errorf("InfoCount = %d, want 1", a.InfoCount)
	}
}

func TestMultipleLogs_countersAccumulate(t *testing.T) {
	a := newTestState()
	a.logPass("p1")
	a.logPass("p2")
	a.logFail("f1")
	a.logWarn("w1")
	a.logWarn("w2")
	a.logWarn("w3")
	if a.PassCount != 2 {
		t.Errorf("PassCount = %d, want 2", a.PassCount)
	}
	if a.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", a.FailCount)
	}
	if a.WarnCount != 3 {
		t.Errorf("WarnCount = %d, want 3", a.WarnCount)
	}
}

// ─── OutputBuffer ────────────────────────────────────────────────────────────

func TestOutputBuffer_containsStrippedMessage(t *testing.T) {
	a := newTestState()
	a.logPass("deployment healthy")
	buf := a.OutputBuffer.String()
	if !strings.Contains(buf, "deployment healthy") {
		t.Errorf("OutputBuffer should contain message, got: %q", buf)
	}
	if strings.Contains(buf, "\033[") {
		t.Error("OutputBuffer should not contain ANSI escape codes")
	}
}

func TestOutputBuffer_failMessage(t *testing.T) {
	a := newTestState()
	a.logFail("service exposed")
	buf := a.OutputBuffer.String()
	if !strings.Contains(buf, "service exposed") {
		t.Errorf("OutputBuffer missing fail message, got: %q", buf)
	}
}

// ─── addFix ──────────────────────────────────────────────────────────────────

func TestAddFix_appendsToFixes(t *testing.T) {
	a := newTestState()
	if len(a.Fixes) != 0 {
		t.Fatal("Fixes should start empty")
	}
	a.addFix("FIX-001", "CRITICAL", "change svc type", "kubectl patch svc ...", nil)
	if len(a.Fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(a.Fixes))
	}
	if a.Fixes[0].ID != "FIX-001" {
		t.Errorf("Fix ID = %q, want FIX-001", a.Fixes[0].ID)
	}
	if a.Fixes[0].Severity != "CRITICAL" {
		t.Errorf("Fix Severity = %q, want CRITICAL", a.Fixes[0].Severity)
	}
}

func TestAddFix_multipleFixesOrdered(t *testing.T) {
	a := newTestState()
	a.addFix("A", "WARNING", "desc A", "", nil)
	a.addFix("B", "CRITICAL", "desc B", "", nil)
	if a.Fixes[0].ID != "A" || a.Fixes[1].ID != "B" {
		t.Error("Fixes should maintain insertion order")
	}
}

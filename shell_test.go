package main

import (
	"strings"
	"testing"
)

// ─── cmdExists ───────────────────────────────────────────────────────────────

func TestCmdExists_knownBinary(t *testing.T) {
	if !cmdExists("ls") {
		t.Error("cmdExists(ls) should be true on macOS/Linux")
	}
}

func TestCmdExists_missingBinary(t *testing.T) {
	if cmdExists("definitely-does-not-exist-xyz-9999") {
		t.Error("cmdExists should return false for non-existent binary")
	}
}

// ─── stripANSI ───────────────────────────────────────────────────────────────

func TestStripANSI_removesColorCodes(t *testing.T) {
	input := "\033[0;32mPASS\033[0m: all good"
	got := stripANSI(input)
	if strings.Contains(got, "\033") {
		t.Errorf("stripANSI did not remove escape codes: %q", got)
	}
	if !strings.Contains(got, "PASS") || !strings.Contains(got, "all good") {
		t.Errorf("stripANSI removed visible content: %q", got)
	}
}

func TestStripANSI_plainString(t *testing.T) {
	input := "no escapes here"
	if got := stripANSI(input); got != input {
		t.Errorf("stripANSI modified plain string: %q -> %q", input, got)
	}
}

func TestStripANSI_multipleSequences(t *testing.T) {
	input := "\033[1m\033[0;31mERROR\033[0m\033[2m detail\033[0m"
	got := stripANSI(input)
	if strings.Contains(got, "\033") {
		t.Errorf("stripANSI left escape codes in: %q", got)
	}
	want := "ERROR detail"
	if got != want {
		t.Errorf("stripANSI: got %q, want %q", got, want)
	}
}

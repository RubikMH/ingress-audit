package main

import "testing"

// ─── extractVersion ─────────────────────────────────────────────────────────

func TestExtractVersion_vTag(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"registry.k8s.io/ingress-nginx/controller:v1.11.0", "v1.11.0"},
		{"registry.k8s.io/ingress-nginx/controller:v1.14.3", "v1.14.3"},
		{"controller:v1.14.3@sha256:abc", "v1.14.3"},
		{"controller:v2.0.0-alpha.1", "v2.0.0"},
		{"", "unknown"},
		{"nodots", "unknown"},
		{"onedot.only", "unknown"},
	}
	for _, c := range cases {
		got := extractVersion(c.input)
		if got != c.want {
			t.Errorf("extractVersion(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestExtractVersion_bareVersion(t *testing.T) {
	// The function splits on '-' taking the prefix; "chart" has no dots so
	// "chart-1.2.3" correctly returns "unknown". A pure digit-dot version works:
	got := extractVersion("some/path:1.2.3")
	if got != "1.2.3" {
		t.Errorf("bare version via colon: got %q, want 1.2.3", got)
	}
	// Prefix before dash that has no dots is not a version
	got2 := extractVersion("chart-1.2.3")
	if got2 != "unknown" {
		t.Errorf("chart-prefixed: got %q, want unknown", got2)
	}
}

// ─── nonEmpty ───────────────────────────────────────────────────────────────

func TestNonEmpty(t *testing.T) {
	cases := []struct {
		input []string
		want  int
	}{
		{[]string{"a", "", "b", "  ", "c"}, 3},
		{[]string{}, 0},
		{[]string{"", " ", "\t"}, 0},
		{[]string{"only"}, 1},
	}
	for _, c := range cases {
		got := nonEmpty(c.input)
		if len(got) != c.want {
			t.Errorf("nonEmpty len = %d, want %d", len(got), c.want)
		}
	}
}

// ─── truncate ───────────────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 3); got != "hel" {
		t.Errorf("truncate short: got %q", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate long: got %q", got)
	}
	if got := truncate("exact", 5); got != "exact" {
		t.Errorf("truncate exact: got %q", got)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("truncate empty: got %q", got)
	}
}

// ─── orDefault ──────────────────────────────────────────────────────────────

func TestOrDefault(t *testing.T) {
	if got := orDefault(""); got != "not-set" {
		t.Errorf("orDefault empty: got %q", got)
	}
	if got := orDefault("foo"); got != "foo" {
		t.Errorf("orDefault non-empty: got %q", got)
	}
}

// ─── getMap ─────────────────────────────────────────────────────────────────

func TestGetMap(t *testing.T) {
	m := map[string]interface{}{
		"nested": map[string]interface{}{"key": "val"},
	}
	got := getMap(m, "nested")
	if got["key"] != "val" {
		t.Errorf("getMap existing key: got %v", got)
	}
	empty := getMap(m, "missing")
	if len(empty) != 0 {
		t.Errorf("getMap missing key should be empty, got %v", empty)
	}
	nilResult := getMap(nil, "key")
	if len(nilResult) != 0 {
		t.Errorf("getMap nil map should be empty, got %v", nilResult)
	}
}

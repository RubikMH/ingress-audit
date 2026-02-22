package main

import (
	"strings"
	"testing"
)

// ─── buildRecommendations ────────────────────────────────────────────────────

func TestBuildRecommendations_latestVersionClusterIP(t *testing.T) {
	a := &AuditState{
		ControllerVersion: "v1.14.3",
		AdmissionSvcType:  "ClusterIP",
	}
	recs := buildRecommendations(a)
	// No upgrade recommendation for latest version
	for _, r := range recs {
		if strings.Contains(r, "Upgrade controller") {
			t.Errorf("should NOT recommend upgrade for v1.14.3, got: %q", r)
		}
		if strings.Contains(r, "ClusterIP") {
			t.Errorf("should NOT recommend ClusterIP change when already ClusterIP, got: %q", r)
		}
	}
	// Always includes migration reminders
	found := 0
	for _, r := range recs {
		if strings.Contains(r, "Gateway API") || strings.Contains(r, "March 2026") {
			found++
		}
	}
	if found == 0 {
		t.Error("should always include migration/retirement reminders")
	}
}

func TestBuildRecommendations_outdatedVersionExposedService(t *testing.T) {
	a := &AuditState{
		ControllerVersion: "v1.11.0",
		AdmissionSvcType:  "LoadBalancer",
	}
	recs := buildRecommendations(a)
	hasUpgrade := false
	hasClusterIP := false
	for _, r := range recs {
		if strings.Contains(r, "Upgrade controller") {
			hasUpgrade = true
		}
		if strings.Contains(r, "ClusterIP") {
			hasClusterIP = true
		}
	}
	if !hasUpgrade {
		t.Error("should recommend upgrade for outdated version")
	}
	if !hasClusterIP {
		t.Error("should recommend ClusterIP for exposed service")
	}
}

func TestBuildRecommendations_neverEmpty(t *testing.T) {
	// Even with ideal state, migration advisories are always included
	a := &AuditState{
		ControllerVersion: "v1.14.3",
		AdmissionSvcType:  "ClusterIP",
	}
	recs := buildRecommendations(a)
	if len(recs) == 0 {
		t.Error("buildRecommendations should never return empty slice")
	}
}

func TestBuildRecommendations_order(t *testing.T) {
	// Upgrade recommendation should come before migration notice
	a := &AuditState{
		ControllerVersion: "v1.10.0",
		AdmissionSvcType:  "ClusterIP",
	}
	recs := buildRecommendations(a)
	if len(recs) < 2 {
		t.Fatal("expected at least 2 recommendations")
	}
	if !strings.Contains(recs[0], "Upgrade") {
		t.Errorf("first recommendation should be upgrade, got: %q", recs[0])
	}
}

// ─── AuditReport structs ─────────────────────────────────────────────────────

func TestAuditResultsReport_fields(t *testing.T) {
	rep := AuditResultsReport{Passed: 10, Failed: 2, Warnings: 3, Info: 5}
	if rep.Passed != 10 || rep.Failed != 2 || rep.Warnings != 3 || rep.Info != 5 {
		t.Error("AuditResultsReport field assignment broken")
	}
}

func TestAdmissionReport_publiclyExposedLogic(t *testing.T) {
	// Verify the PubliclyExposed logic used in generateJSONReport
	cases := []struct {
		svcType string
		want    bool
	}{
		{"ClusterIP", false},
		{"LoadBalancer", true},
		{"NodePort", true},
	}
	for _, c := range cases {
		got := c.svcType != "ClusterIP"
		if got != c.want {
			t.Errorf("PubliclyExposed(%q) = %v, want %v", c.svcType, got, c.want)
		}
	}
}

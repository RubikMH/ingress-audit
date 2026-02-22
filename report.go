package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// JSON report structs
// ─────────────────────────────────────────────

// AuditReport is the top-level JSON report written to disk after each audit.
type AuditReport struct {
	AuditTimestamp  string             `json:"audit_timestamp"`
	Domain          string             `json:"domain"`
	AdminEmail      string             `json:"admin_email"`
	ClusterVersion  string             `json:"cluster_version"`
	Namespace       string             `json:"namespace"`
	Controller      ControllerReport   `json:"controller"`
	Admission       AdmissionReport    `json:"admission_controller"`
	Security        SecurityReport     `json:"security"`
	AuditResults    AuditResultsReport `json:"audit_results"`
	Recommendations []string           `json:"recommendations"`
}

// ControllerReport holds version and deployment details for the controller.
type ControllerReport struct {
	DeploymentType string `json:"deployment_type"`
	Version        string `json:"version"`
	Image          string `json:"image"`
	LatestVersion  string `json:"latest_version"`
}

// AdmissionReport describes the admission controller's network exposure.
type AdmissionReport struct {
	ServiceType     string `json:"service_type"`
	ClusterIP       string `json:"cluster_ip"`
	ExternalIP      string `json:"external_ip"`
	PubliclyExposed bool   `json:"publicly_exposed"`
}

// SecurityReport holds aggregated security findings.
type SecurityReport struct {
	AbuseBSICompliant         bool `json:"abusebsi_compliant"`
	SnippetAnnotationsEnabled bool `json:"snippet_annotations_enabled"`
	NetworkPoliciesCount      int  `json:"network_policies_count"`
}

// AuditResultsReport summarises pass/fail/warn/info counters.
type AuditResultsReport struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

// ─────────────────────────────────────────────
// Report generation
// ─────────────────────────────────────────────

// generateJSONReport serialises the current AuditState to a JSON file.
func (a *AuditState) generateJSONReport() {
	recs := buildRecommendations(a)

	report := AuditReport{
		AuditTimestamp: time.Now().UTC().Format(time.RFC3339),
		Domain:         a.Domain,
		AdminEmail:     a.Email,
		ClusterVersion: a.ClusterVersion,
		Namespace:      a.Namespace,
		Controller: ControllerReport{
			DeploymentType: a.DeploymentType,
			Version:        a.ControllerVersion,
			Image:          a.ControllerImage,
			LatestVersion:  "v1.14.3",
		},
		Admission: AdmissionReport{
			ServiceType:     a.AdmissionSvcType,
			ClusterIP:       a.AdmissionClusterIP,
			ExternalIP:      a.AdmissionExternalIP,
			PubliclyExposed: a.AdmissionSvcType != "ClusterIP",
		},
		Security: SecurityReport{
			AbuseBSICompliant:         a.AdmissionSvcType == "ClusterIP",
			SnippetAnnotationsEnabled: a.AllowSnippets == "true",
			NetworkPoliciesCount:      a.NpCount,
		},
		AuditResults: AuditResultsReport{
			Passed:   a.PassCount,
			Failed:   a.FailCount,
			Warnings: a.WarnCount,
			Info:     a.InfoCount,
		},
		Recommendations: recs,
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(a.JSONReportFile, data, 0644)
}

// buildRecommendations derives a prioritised list of action items from state.
func buildRecommendations(a *AuditState) []string {
	var recs []string
	if a.ControllerVersion != "v1.14.3" {
		recs = append(recs, "Upgrade controller to v1.14.3")
	}
	if a.AdmissionSvcType != "ClusterIP" {
		recs = append(recs, "Change admission controller service to ClusterIP")
	}
	recs = append(recs, "Plan migration from Ingress-NGINX (retiring March 2026)")
	recs = append(recs, "Consider migrating to Gateway API or alternative controller")
	return recs
}

// ─────────────────────────────────────────────
// Console summary
// ─────────────────────────────────────────────

// generateSummary prints the final human-readable audit summary.
func (a *AuditState) generateSummary() {
	a.printHeader("AUDIT SUMMARY")

	// ── Stats box ────────────────────────────────────
	summaryBoxColor := lipgloss.Color("46") // green
	if a.FailCount > 0 {
		summaryBoxColor = lipgloss.Color("196") // red
	} else if a.WarnCount > 0 {
		summaryBoxColor = lipgloss.Color("226") // yellow
	}
	summaryBox := infoBox(summaryBoxColor,
		[][]string{
			{"Domain:", a.Domain},
			{"Email:", a.Email},
		},
		[][]string{
			{"✓ Passed:", fmt.Sprintf("%d", a.PassCount)},
			{"✗ Failed:", fmt.Sprintf("%d", a.FailCount)},
			{"⚠ Warnings:", fmt.Sprintf("%d", a.WarnCount)},
			{"ℹ Info:", fmt.Sprintf("%d", a.InfoCount)},
		},
	)
	for _, line := range strings.Split(summaryBox, "\n") {
		a.writeln("  " + line)
	}
	a.writeln("")

	// ── Overall status ───────────────────────────────
	switch {
	case a.FailCount == 0 && a.WarnCount == 0:
		a.writeln(fmt.Sprintf("  %s%s● Overall Status: EXCELLENT%s", Green, Bold, Reset))
		a.writeln("  No critical issues or warnings found.")
	case a.FailCount == 0:
		a.writeln(fmt.Sprintf("  %s%s● Overall Status: GOOD%s", Yellow, Bold, Reset))
		a.writeln(fmt.Sprintf("  No critical issues, but %d warning(s) found.", a.WarnCount))
	case a.FailCount <= 2:
		a.writeln(fmt.Sprintf("  %s%s● Overall Status: NEEDS ATTENTION%s", Yellow, Bold, Reset))
		a.writeln(fmt.Sprintf("  %d critical issue(s) found — remediation recommended.", a.FailCount))
	default:
		a.writeln(fmt.Sprintf("  %s%s● Overall Status: CRITICAL%s", Red, Bold, Reset))
		a.writeln(fmt.Sprintf("  %d critical issue(s) found — immediate action required!", a.FailCount))
	}

	a.writeln("")
	a.writeln(fmt.Sprintf("  %sKey Findings:%s", Bold, Reset))
	a.writeln(fmt.Sprintf("    • Controller version:          %s", a.ControllerVersion))
	a.writeln(fmt.Sprintf("    • Admission controller:        %s", a.AdmissionSvcType))
	compliant := fmt.Sprintf("%s✓ COMPLIANT%s", Green, Reset)
	if a.AdmissionSvcType != "ClusterIP" {
		compliant = fmt.Sprintf("%s✗ NON-COMPLIANT%s", Red, Reset)
	}
	a.writeln(fmt.Sprintf("    • AbuseBSI compliance:         %s", compliant))

	a.writeln("")
	a.writeln(fmt.Sprintf("  %sRecommendations:%s", Bold, Reset))
	for i, rec := range buildRecommendations(a) {
		a.writeln(fmt.Sprintf("    %d. %s", i+1, rec))
	}

	a.writeln("")
	a.writeln(fmt.Sprintf("  %sAbuseBSI Report Response:%s", Bold, Reset))
	a.writeln("    Report ID: CB-Report#20260218-10009947")
	if a.AdmissionSvcType == "ClusterIP" {
		a.writeln(fmt.Sprintf("    Status:    %s✓ RESOLVED%s", Green, Reset))
		a.writeln("    Details:   Admission controller is not publicly exposed")
	} else {
		a.writeln(fmt.Sprintf("    Status:    %s✗ STILL VULNERABLE%s", Red, Reset))
		a.writeln(fmt.Sprintf("    Details:   Exposed via %s — immediate remediation required", a.AdmissionSvcType))
	}

	a.writeln("")
	a.writeln(fmt.Sprintf("  %sReport files generated:%s", Bold, Reset))
	a.writeln(fmt.Sprintf("    • %s (text)", a.TextReportFile))
	a.writeln(fmt.Sprintf("    • %s (json)", a.JSONReportFile))
	a.writeln("")
	a.writeln(fmt.Sprintf("  %sFor questions: %s%s", Bold, a.Email, Reset))
}

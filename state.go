package main

import (
	"bytes"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
// Domain types
// ─────────────────────────────────────────────

// Fix represents a single remediable issue found during the audit.
type Fix struct {
	ID          string
	Severity    string // "CRITICAL" | "WARNING"
	Description string
	Command     string // printed to the user before execution
	Run         func() error
}

// AuditState carries all configuration, discovered values, counters and the
// collected output for both the terminal and the saved text report.
type AuditState struct {
	// ── User config ──────────────────────────────────
	Namespace      string
	Namespaces     []string
	ScanAll        bool
	Domain         string
	Email          string
	ControllerName string

	// ── Fixable issues ────────────────────────────────
	Fixes []Fix

	// ── Result counters ───────────────────────────────
	PassCount int
	WarnCount int
	FailCount int
	InfoCount int

	// ── Discovered cluster values ─────────────────────
	ClusterVersion      string
	APIServer           string
	CurrentContext      string
	NodeCount           int
	ReadyNodes          int
	DeploymentType      string
	ControllerImage     string
	ControllerVersion   string
	ControllerReplicas  string
	HelmChart           string
	HelmChartVersion    string
	HelmStatus          string
	HelmRevision        string
	AdmissionSvcType    string
	AdmissionClusterIP  string
	AdmissionExternalIP string
	IngressExposing     string
	AllowSnippets       string
	CPULimit            string
	MemoryLimit         string
	CPURequest          string
	MemoryRequest       string
	NpCount             int
	UpdateStrategy      string
	ImagePullPolicy     string

	// ── Report file paths ─────────────────────────────
	TextReportFile string
	JSONReportFile string

	// ── Dual-write output buffer (terminal + file) ────
	OutputBuffer bytes.Buffer
}

// ─────────────────────────────────────────────
// Output / logging helpers
// ─────────────────────────────────────────────

// write prints s to stdout and appends the ANSI-stripped version to the
// output buffer that is later saved as the text report.
func (a *AuditState) write(s string) {
	fmt.Print(s)
	a.OutputBuffer.WriteString(stripANSI(s))
}

func (a *AuditState) writeln(s string) { a.write(s + "\n") }

func (a *AuditState) logPass(msg string) {
	a.writeln(fmt.Sprintf("%s✓ PASS%s: %s", Green, Reset, msg))
	a.PassCount++
}

func (a *AuditState) logFail(msg string) {
	a.writeln(fmt.Sprintf("%s✗ FAIL%s: %s", Red, Reset, msg))
	a.FailCount++
}

func (a *AuditState) logWarn(msg string) {
	a.writeln(fmt.Sprintf("%s⚠ WARN%s: %s", Yellow, Reset, msg))
	a.WarnCount++
}

func (a *AuditState) logInfo(msg string) {
	a.writeln(fmt.Sprintf("%sℹ INFO%s: %s", Blue, Reset, msg))
	a.InfoCount++
}

func (a *AuditState) logStep(msg string) {
	a.writeln(fmt.Sprintf("  %s→%s %s", Cyan, Reset, msg))
}

// addFix registers a remediable issue to be offered at the end of the audit.
func (a *AuditState) addFix(id, severity, description, command string, run func() error) {
	a.Fixes = append(a.Fixes, Fix{
		ID:          id,
		Severity:    severity,
		Description: description,
		Command:     command,
		Run:         run,
	})
}

// ─────────────────────────────────────────────
// Section / header printers
// ─────────────────────────────────────────────

func (a *AuditState) printHeader(title string) {
	line := strings.Repeat("═", 60)
	a.writeln(fmt.Sprintf("\n%s%s%s", Blue+Bold, line, Reset))
	a.writeln(fmt.Sprintf("%s%s  %-56s%s", Blue+Bold, "  ", title, Reset))
	a.writeln(fmt.Sprintf("%s%s%s\n", Blue+Bold, line, Reset))
}

func (a *AuditState) printSection(title string) {
	a.writeln(fmt.Sprintf("\n%s▶ %s%s", Bold, title, Reset))
	a.writeln(strings.Repeat("─", 55))
}

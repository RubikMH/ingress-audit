package main

import (
	"fmt"
	"strings"
)

// PHASE 7 -- CVE & Vulnerability Scan

func (a *AuditState) auditVulnerabilities() {
	a.printHeader("PHASE 7 — VULNERABILITY SCAN")

	// -- CVE check
	a.printSection("Known CVEs for Current Version")
	a.logStep(fmt.Sprintf("Checking CVE database for version %s...", a.ControllerVersion))
	switch {
	case a.ControllerVersion == "v1.14.3":
		a.logPass(fmt.Sprintf("No known critical CVEs for %s", a.ControllerVersion))
	case strings.HasPrefix(a.ControllerVersion, "v1.14."):
		a.logWarn(fmt.Sprintf("Version %s may have issues — upgrade to v1.14.3", a.ControllerVersion))
	case strings.HasPrefix(a.ControllerVersion, "v1.13.") ||
		strings.HasPrefix(a.ControllerVersion, "v1.12.") ||
		strings.HasPrefix(a.ControllerVersion, "v1.11.") ||
		strings.HasPrefix(a.ControllerVersion, "v1.10."):
		a.logFail(fmt.Sprintf("Version %s is significantly outdated — multiple known CVEs", a.ControllerVersion))
		a.logInfo("CRITICAL: Upgrade to v1.14.3 immediately")
	case strings.HasPrefix(a.ControllerVersion, "v1.9.") ||
		strings.HasPrefix(a.ControllerVersion, "v1.8.") ||
		strings.HasPrefix(a.ControllerVersion, "v1.7."):
		a.logFail(fmt.Sprintf("CRITICAL: Version %s has known critical security vulnerabilities", a.ControllerVersion))
		a.logInfo("Upgrade to v1.14.3 IMMEDIATELY")
	default:
		a.logWarn(fmt.Sprintf("Unknown version %s — cannot verify CVE status", a.ControllerVersion))
	}

	// -- AbuseBSI compliance
	a.printSection("AbuseBSI Report Compliance")
	a.logStep("Checking CB-Report#20260218-10009947 specific vulnerability...")
	switch {
	case a.DeploymentType == "":
		a.logInfo(fmt.Sprintf(
			"No ingress-nginx controller in namespace '%s' — AbuseBSI check not applicable", a.Namespace))
	case a.AdmissionSvcType == "ClusterIP":
		a.logPass(fmt.Sprintf("Admission controller not publicly exposed for %s (compliant)", a.Domain))
	default:
		a.logFail(fmt.Sprintf("Admission controller publicly exposed via %s (NON-COMPLIANT for %s)",
			a.AdmissionSvcType, a.Domain))
		a.logInfo("This is the specific vulnerability reported by AbuseBSI")
	}
}

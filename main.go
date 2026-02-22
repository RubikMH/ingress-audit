package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	a := &AuditState{}
	interactiveSetup(a)
	if a.ScanAll && len(a.Namespaces) > 1 {
		runMultiNamespaceScan(a)
		return
	}
	runAudit(a)
	if a.FailCount > 0 {
		os.Exit(1)
	}
}

func runAudit(a *AuditState) {
	a.auditPreflight()
	a.auditVersion()
	a.auditAdmissionController()
	a.auditNetworkSecurity()
	a.auditConfiguration()
	a.auditPodSecurity()
	a.auditVulnerabilities()
	a.auditCertificates()
	a.auditIngressResources()
	a.generateJSONReport()
	a.generateSummary()
	_ = os.WriteFile(a.TextReportFile, a.OutputBuffer.Bytes(), 0644)
	if len(a.Fixes) > 0 {
		offerFixes(a)
	}
}

func runMultiNamespaceScan(a *AuditState) {
	totalFail := 0
	ts := time.Now().Format("20060102-150405")
	for i, ns := range a.Namespaces {
		ns := ns
		sub := &AuditState{
			Namespace:      ns,
			Namespaces:     []string{ns},
			Domain:         a.Domain,
			Email:          a.Email,
			ControllerName: a.ControllerName,
			TextReportFile: fmt.Sprintf("ingress-audit-%s-%s.txt", ns, ts),
			JSONReportFile: fmt.Sprintf("ingress-audit-%s-%s.json", ns, ts),
		}
		fmt.Printf("\n%s--- NAMESPACE %d/%d: %s ---%s\n",
			Bold+Blue, i+1, len(a.Namespaces), ns, Reset)
		if !nsHasIngressNginx(ns, a.ControllerName) {
			fmt.Printf("  %sSKIP%s: No ingress-nginx in namespace %s%s%s\n",
				Yellow, Reset, Cyan, ns, Reset)
			continue
		}
		runAudit(sub)
		totalFail += sub.FailCount
	}
	fmt.Printf("\n%s=== MULTI-NAMESPACE SCAN COMPLETE ===%s\n", Bold+Blue, Reset)
	fmt.Printf("  Namespaces scanned: %s%d%s\n", Cyan, len(a.Namespaces), Reset)
	for _, ns := range a.Namespaces {
		fmt.Printf("  * %s%s%s\n", Cyan, ns, Reset)
	}
	if totalFail > 0 {
		fmt.Printf("\n  %s%sX Total failures: %d%s\n", Red, Bold, totalFail, Reset)
		os.Exit(1)
	}
	fmt.Printf("\n  %s%s✓ All namespaces passed%s\n", Green, Bold, Reset)
}

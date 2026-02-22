package main

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
// PHASE 6 — Pod Security Audit
// ─────────────────────────────────────────────

func (a *AuditState) auditPodSecurity() {
	a.printHeader("PHASE 6 — POD SECURITY AUDIT")

	// ── Running pods ─────────────────────────────────
	a.printSection("Running Pods")
	a.logStep("Listing ingress-nginx pods...")

	podOut, _ := kubectl("get", "pods", "-n", a.Namespace,
		"-l", "app.kubernetes.io/name=ingress-nginx", "--no-headers")
	podLines := nonEmpty(strings.Split(podOut, "\n"))
	podCount := len(podLines)
	readyCount := 0
	for _, l := range podLines {
		if strings.Contains(l, "1/1") || strings.Contains(l, "Running") {
			readyCount++
		}
	}

	a.logInfo(fmt.Sprintf("Total pods: %d", podCount))
	a.logInfo(fmt.Sprintf("Ready pods: %d", readyCount))
	switch {
	case podCount > 0 && podCount == readyCount:
		a.logPass("All ingress-nginx pods are ready")
	case podCount > 0:
		a.logWarn(fmt.Sprintf("Some pods not ready (%d/%d)", readyCount, podCount))
	default:
		a.logFail("No ingress-nginx pods found")
	}

	// ── Security context ─────────────────────────────
	a.printSection("Security Context")
	resType := strings.ToLower(a.DeploymentType)
	if resType == "" {
		resType = "deployment"
	}

	a.logStep("Checking runAsNonRoot...")
	runAsNonRoot, _ := kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.securityContext.runAsNonRoot}")
	runAsUser, _ := kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.securityContext.runAsUser}")

	a.logInfo(fmt.Sprintf("runAsNonRoot: %s", orDefault(runAsNonRoot)))
	a.logInfo(fmt.Sprintf("runAsUser:    %s", orDefault(runAsUser)))

	switch {
	case runAsNonRoot == "true":
		a.logPass("Running as non-root user")
	case runAsUser != "" && runAsUser != "0":
		a.logPass(fmt.Sprintf("Running as user %s (non-root)", runAsUser))
	default:
		a.logWarn("Security context should enforce non-root execution")
	}

	a.logStep("Checking for privileged containers...")
	privileged, _ := kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.containers[*].securityContext.privileged}")
	if privileged == "true" {
		a.logWarn("Container running in privileged mode")
	} else {
		a.logPass("Container not running in privileged mode")
	}
}

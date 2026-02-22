package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
// PHASE 4 — Network Security Audit
// ─────────────────────────────────────────────

func (a *AuditState) auditNetworkSecurity() {
	a.printHeader("PHASE 4 — NETWORK SECURITY AUDIT")

	// ── Controller service exposure ──────────────────
	a.printSection("Controller Service Exposure")
	a.logStep("Fetching controller service type...")

	ctrlSvcType, err := kubectl("get", "svc", "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.type}")
	if err != nil {
		a.logWarn("Controller service not found")
	} else {
		a.logInfo(fmt.Sprintf("Controller service type: %s", ctrlSvcType))
		switch ctrlSvcType {
		case "LoadBalancer":
			extIP, _ := kubectl("get", "svc", "-n", a.Namespace, a.ControllerName,
				"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
			a.logInfo(fmt.Sprintf("External IP: %s", extIP))
			a.logPass("Controller properly exposed via LoadBalancer (expected for ingress)")
		case "NodePort":
			httpPort, _ := kubectl("get", "svc", "-n", a.Namespace, a.ControllerName,
				"-o", `jsonpath={.spec.ports[?(@.name=="http")].nodePort}`)
			httpsPort, _ := kubectl("get", "svc", "-n", a.Namespace, a.ControllerName,
				"-o", `jsonpath={.spec.ports[?(@.name=="https")].nodePort}`)
			a.logInfo(fmt.Sprintf("HTTP NodePort:  %s", httpPort))
			a.logInfo(fmt.Sprintf("HTTPS NodePort: %s", httpsPort))
			a.logPass("Controller exposed via NodePort (common for bare-metal)")
		case "ClusterIP":
			a.logWarn("Controller is ClusterIP — confirm external access is handled elsewhere")
		}
	}

	// ── NetworkPolicy check ──────────────────────────
	a.printSection("Network Policy Check")
	a.logStep("Checking for NetworkPolicies...")

	npOut, _ := kubectl("get", "networkpolicies", "-n", a.Namespace, "--no-headers")
	npLines := nonEmpty(strings.Split(npOut, "\n"))
	a.NpCount = len(npLines)
	if a.NpCount > 0 {
		a.logPass(fmt.Sprintf("Found %d NetworkPolicy resource(s)", a.NpCount))
		for _, l := range npLines {
			a.writeln(fmt.Sprintf("    %s", l))
		}
	} else {
		a.logWarn("No NetworkPolicies found — consider adding for defense-in-depth")
	}

	// ── Cluster-wide external services ───────────────
	a.printSection("All External Services (Cluster-wide)")
	a.logStep("Scanning for LoadBalancer/NodePort services...")

	svcJSON, _ := kubectl("get", "svc", "-A", "-o", "json")
	if svcJSON != "" {
		var svcList map[string]interface{}
		if json.Unmarshal([]byte(svcJSON), &svcList) == nil {
			items, _ := svcList["items"].([]interface{})
			found := 0
			for _, item := range items {
				im, _ := item.(map[string]interface{})
				meta := getMap(im, "metadata")
				spec := getMap(im, "spec")
				t := fmt.Sprintf("%v", spec["type"])
				if t == "LoadBalancer" || t == "NodePort" {
					ns := fmt.Sprintf("%v", meta["namespace"])
					name := fmt.Sprintf("%v", meta["name"])
					a.logInfo(fmt.Sprintf("  %s/%s (%s)", ns, name, t))
					found++
				}
			}
			if found == 0 {
				a.logInfo("No LoadBalancer or NodePort services found")
			}
		}
	}
}

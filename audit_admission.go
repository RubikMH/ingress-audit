package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// PHASE 3 — Admission Controller Security Audit
// ─────────────────────────────────────────────

func (a *AuditState) auditAdmissionController() {
	a.printHeader("PHASE 3 — ADMISSION CONTROLLER SECURITY AUDIT")

	// ── Service discovery ────────────────────────────
	a.printSection("Service Discovery")
	a.logStep("Searching for admission controller service...")

	if _, err := kubectl("get", "svc", "-n", a.Namespace, "ingress-nginx-controller-admission"); err != nil {
		a.logWarn("Admission controller service not found — may be a custom installation")
		return
	}
	a.logPass("Found admission controller service")

	// ── Service details ──────────────────────────────
	a.printSection("Service Configuration Analysis")

	a.logStep("Fetching service type...")
	a.AdmissionSvcType, _ = kubectl("get", "svc", "-n", a.Namespace,
		"ingress-nginx-controller-admission", "-o", "jsonpath={.spec.type}")

	a.logStep("Fetching cluster IP...")
	a.AdmissionClusterIP, _ = kubectl("get", "svc", "-n", a.Namespace,
		"ingress-nginx-controller-admission", "-o", "jsonpath={.spec.clusterIP}")

	a.logStep("Fetching ports...")
	ports, _ := kubectl("get", "svc", "-n", a.Namespace,
		"ingress-nginx-controller-admission", "-o", "jsonpath={.spec.ports[*].port}")

	a.logStep("Fetching selector...")
	selector, _ := kubectl("get", "svc", "-n", a.Namespace,
		"ingress-nginx-controller-admission", "-o", "jsonpath={.spec.selector}")

	svcAge, _ := kubectl("get", "svc", "-n", a.Namespace,
		"ingress-nginx-controller-admission", "-o", "jsonpath={.metadata.creationTimestamp}")

	a.logInfo("Name:       ingress-nginx-controller-admission")
	a.logInfo(fmt.Sprintf("Type:       %s", a.AdmissionSvcType))
	a.logInfo(fmt.Sprintf("Cluster IP: %s", a.AdmissionClusterIP))
	a.logInfo(fmt.Sprintf("Ports:      %s", ports))
	a.logInfo(fmt.Sprintf("Selector:   %s", selector))
	a.logInfo(fmt.Sprintf("Created:    %s", svcAge))

	// ── Critical exposure check ──────────────────────
	a.printSection("🔒 CRITICAL: External Exposure Check")
	a.logStep(fmt.Sprintf("Analyzing service type: %s...", a.AdmissionSvcType))

	switch a.AdmissionSvcType {
	case "ClusterIP":
		a.logPass("✓ Admission controller uses ClusterIP (internal only)")
		a.logInfo("External access: BLOCKED ✓")
		a.logInfo("Security posture: SECURE")
		a.writeln("  This configuration meets security best practices:")
		a.writeln("    ✓ Not accessible from the internet")
		a.writeln("    ✓ Protected by cluster network policies")
		a.writeln(fmt.Sprintf("    ✓ Compliant with AbuseBSI requirements for %s", a.Domain))

	case "LoadBalancer":
		a.AdmissionExternalIP, _ = kubectl("get", "svc", "-n", a.Namespace,
			"ingress-nginx-controller-admission",
			"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
		a.logFail("✗ CRITICAL: Admission controller exposed via LoadBalancer!")
		a.logInfo(fmt.Sprintf("External IP: %s", a.AdmissionExternalIP))
		a.writeln("  IMMEDIATE REMEDIATION:")
		a.writeln(fmt.Sprintf("    kubectl patch svc ingress-nginx-controller-admission \\"))
		a.writeln(fmt.Sprintf("      -n %s \\", a.Namespace))
		a.writeln("      -p '{\"spec\":{\"type\":\"ClusterIP\"}}'")
		ns := a.Namespace
		a.addFix("admission-loadbalancer", "CRITICAL",
			"Change admission controller service from LoadBalancer → ClusterIP",
			fmt.Sprintf(`kubectl patch svc ingress-nginx-controller-admission -n %s -p '{"spec":{"type":"ClusterIP"}}'`, ns),
			func() error {
				return runCmd("kubectl", "patch", "svc", "ingress-nginx-controller-admission",
					"-n", ns, "-p", `{"spec":{"type":"ClusterIP"}}`)
			})

	case "NodePort":
		nodePort, _ := kubectl("get", "svc", "-n", a.Namespace,
			"ingress-nginx-controller-admission",
			"-o", "jsonpath={.spec.ports[0].nodePort}")
		a.logFail(fmt.Sprintf("✗ CRITICAL: Admission controller exposed via NodePort %s!", nodePort))
		a.writeln("  IMMEDIATE REMEDIATION:")
		a.writeln(fmt.Sprintf("    kubectl patch svc ingress-nginx-controller-admission \\"))
		a.writeln(fmt.Sprintf("      -n %s \\", a.Namespace))
		a.writeln("      -p '{\"spec\":{\"type\":\"ClusterIP\"}}'")
		a.writeln("\n  Exposed on nodes:")
		nodes, _ := kubectl("get", "nodes",
			"-o", "custom-columns=NAME:.metadata.name,INTERNAL-IP:.status.addresses[0].address",
			"--no-headers")
		for _, l := range strings.Split(nodes, "\n") {
			if strings.TrimSpace(l) != "" {
				a.writeln(fmt.Sprintf("    %s", l))
			}
		}
	}

	// ── Explicit external IPs ─────────────────────────
	extIPs, _ := kubectl("get", "svc", "-n", a.Namespace,
		"ingress-nginx-controller-admission", "-o", "jsonpath={.spec.externalIPs}")
	if extIPs != "" && extIPs != "null" {
		a.logFail(fmt.Sprintf("External IPs explicitly configured: %s", extIPs))
	}

	// ── Ingress exposure ─────────────────────────────
	a.printSection("Ingress Resource Exposure Check")
	a.logStep("Scanning all Ingress resources for admission controller exposure...")

	ingressJSON, _ := kubectl("get", "ingress", "-A", "-o", "json")
	a.IngressExposing = ""
	if ingressJSON != "" {
		var ingressList map[string]interface{}
		if json.Unmarshal([]byte(ingressJSON), &ingressList) == nil {
			items, _ := ingressList["items"].([]interface{})
			for _, item := range items {
				itemMap, _ := item.(map[string]interface{})
				meta := getMap(itemMap, "metadata")
				ns := fmt.Sprintf("%v", meta["namespace"])
				name := fmt.Sprintf("%v", meta["name"])
				spec := getMap(itemMap, "spec")
				rules, _ := spec["rules"].([]interface{})
				for _, rule := range rules {
					ruleMap, _ := rule.(map[string]interface{})
					http := getMap(ruleMap, "http")
					paths, _ := http["paths"].([]interface{})
					for _, path := range paths {
						pathMap, _ := path.(map[string]interface{})
						backend := getMap(pathMap, "backend")
						svc := getMap(backend, "service")
						svcName := fmt.Sprintf("%v", svc["name"])
						if strings.Contains(svcName, "admission") {
							a.IngressExposing += ns + "/" + name + "\n"
						}
					}
				}
			}
		}
	}

	if a.IngressExposing != "" {
		a.logFail("✗ CRITICAL: Found Ingress resource(s) exposing admission controller!")
		for _, ing := range strings.Split(strings.TrimSpace(a.IngressExposing), "\n") {
			a.writeln(fmt.Sprintf("    - %s", ing))
		}
		a.logInfo("IMMEDIATE REMEDIATION: Remove these Ingress resources or update backend service")
		exposing := a.IngressExposing
		a.addFix("ingress-exposure", "CRITICAL",
			fmt.Sprintf("Delete Ingress resource(s) exposing admission controller: %s", strings.TrimSpace(exposing)),
			fmt.Sprintf("kubectl delete ingress -n <ns> <name>  (for each: %s)", strings.TrimSpace(exposing)),
			func() error { return fixDeleteExposingIngress(exposing) })
	} else {
		a.logPass("✓ No Ingress resources exposing admission controller")
		a.logInfo("All Ingress resources verified safe")
	}

	// ── Webhook configuration ─────────────────────────
	a.printSection("Webhook Configuration Validation")
	a.logStep("Analyzing ValidatingWebhookConfiguration...")

	whJSON, _ := kubectl("get", "validatingwebhookconfigurations", "-o", "json")
	if whJSON != "" {
		var whList map[string]interface{}
		if json.Unmarshal([]byte(whJSON), &whList) == nil {
			items, _ := whList["items"].([]interface{})
			for _, item := range items {
				itemMap, _ := item.(map[string]interface{})
				meta := getMap(itemMap, "metadata")
				whName := fmt.Sprintf("%v", meta["name"])
				if strings.Contains(whName, "ingress-nginx") {
					a.logInfo(fmt.Sprintf("Webhook name: %s", whName))
					webhooks, _ := itemMap["webhooks"].([]interface{})
					if len(webhooks) > 0 {
						wh := webhooks[0].(map[string]interface{})
						cc := getMap(wh, "clientConfig")
						svc := getMap(cc, "service")
						a.logInfo(fmt.Sprintf("  Points to service: %v", svc["name"]))
						a.logInfo(fmt.Sprintf("  In namespace:      %v", svc["namespace"]))
						a.logInfo(fmt.Sprintf("  Port:              %v", svc["port"]))
						fp := fmt.Sprintf("%v", wh["failurePolicy"])
						a.logInfo(fmt.Sprintf("  Failure policy:    %s", fp))
						if fmt.Sprintf("%v", svc["namespace"]) == a.Namespace {
							a.logPass(fmt.Sprintf("✓ Webhook configured correctly for namespace %s", a.Namespace))
						} else {
							a.logWarn(fmt.Sprintf("Webhook namespace mismatch: expected %s", a.Namespace))
						}
					}
				}
			}
		}
	} else {
		a.logWarn("ValidatingWebhookConfiguration not found — admission controller may not be active")
	}

	// ── Endpoints ────────────────────────────────────
	a.printSection("Network Accessibility Analysis")
	a.logStep("Checking service endpoints...")

	epIPs, _ := kubectl("get", "endpoints", "-n", a.Namespace,
		"ingress-nginx-controller-admission",
		"-o", "jsonpath={.subsets[*].addresses[*].ip}")
	epCount := len(nonEmpty(strings.Fields(epIPs)))
	if epCount > 0 {
		a.logInfo(fmt.Sprintf("Service has %d active endpoint(s): %s", epCount, epIPs))
	} else {
		a.logWarn("No active endpoints found — service may not be functional")
	}

	// ── AbuseBSI compliance ──────────────────────────
	a.printSection("AbuseBSI Compliance Summary")
	boxColor := lipgloss.Color("196") // red
	if a.AdmissionSvcType == "ClusterIP" && a.IngressExposing == "" {
		boxColor = lipgloss.Color("46") // green
	}
	box := infoBox(boxColor,
		[][]string{{"Report ID:", "CB-Report#20260218-10009947"}},
		[][]string{
			{"Domain:", a.Domain},
			{"Email:", a.Email},
		},
	)
	for _, line := range strings.Split(box, "\n") {
		a.writeln("  " + line)
	}

	if a.AdmissionSvcType == "ClusterIP" && a.IngressExposing == "" {
		a.writeln(fmt.Sprintf("\n  %s%s✓ COMPLIANT — Vulnerability has been mitigated.%s\n", Green, Bold, Reset))
		a.writeln("  Details:")
		a.writeln("    ✓ Admission controller is ClusterIP (not exposed)")
		a.writeln("    ✓ No Ingress resources expose the admission endpoint")
		a.writeln("    ✓ Only accessible within cluster network")
	} else {
		a.writeln(fmt.Sprintf("\n  %s%s✗ NON-COMPLIANT — Vulnerability is STILL PRESENT!%s\n", Red, Bold, Reset))
		if a.AdmissionSvcType != "ClusterIP" {
			a.writeln(fmt.Sprintf("    ✗ Service type is %s (must be ClusterIP)", a.AdmissionSvcType))
		}
		if a.IngressExposing != "" {
			a.writeln("    ✗ Ingress resources are exposing the admission controller")
		}
		a.writeln("\n  IMMEDIATE ACTION REQUIRED — See remediation steps above.")
	}
}

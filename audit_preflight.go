package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ─────────────────────────────────────────────
// PHASE 1 — Pre-flight Checks
// ─────────────────────────────────────────────

func (a *AuditState) auditPreflight() {
	a.printHeader("PHASE 1 — PRE-FLIGHT CHECKS")

	// ── Required tools ──────────────────────────────
	a.printSection("Required Tools Validation")

	type toolDef struct {
		name    string
		version func() string
	}
	tools := []toolDef{
		{"kubectl", func() string {
			v, _ := kubectl("version", "--client", "--short")
			for _, line := range strings.Split(v, "\n") {
				if strings.Contains(line, "Client") {
					parts := strings.Fields(line)
					if len(parts) > 2 {
						return parts[2]
					}
				}
			}
			return "unknown"
		}},
		{"helm", func() string {
			v, _ := helmCmd("version", "--short")
			if idx := strings.Index(v, "+"); idx != -1 {
				return v[:idx]
			}
			return v
		}},
		{"jq", func() string {
			out, _ := exec.Command("jq", "--version").Output()
			return strings.TrimSpace(string(out))
		}},
	}

	for _, t := range tools {
		a.logStep(fmt.Sprintf("Checking %s...", t.name))
		if cmdExists(t.name) {
			a.logPass(fmt.Sprintf("%s is installed (%s)", t.name, t.version()))
		} else {
			a.logFail(fmt.Sprintf("%s is not installed — please install it and retry", t.name))
			os.Exit(1)
		}
	}

	// ── Cluster connectivity ─────────────────────────
	a.printSection("Kubernetes Cluster Connectivity")
	a.logStep("Executing: kubectl cluster-info...")

	if _, _, err := kubectlE("cluster-info"); err != nil {
		a.logFail("Cannot reach Kubernetes API server — check kubeconfig")
		os.Exit(1)
	}
	a.logPass("Kubernetes cluster is reachable")

	a.logStep("Fetching cluster version...")
	v, _ := kubectl("version", "--short")
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "Server") {
			parts := strings.Fields(line)
			if len(parts) > 2 {
				a.ClusterVersion = parts[2]
			}
		}
	}
	a.logInfo(fmt.Sprintf("Cluster version: %s", a.ClusterVersion))

	a.logStep("Getting API server endpoint...")
	info, _ := kubectl("cluster-info")
	for _, line := range strings.Split(info, "\n") {
		if strings.Contains(line, "control plane") {
			parts := strings.Fields(line)
			a.APIServer = parts[len(parts)-1]
		}
	}
	a.logInfo(fmt.Sprintf("API Server: %s", a.APIServer))

	a.logStep("Checking current context...")
	a.CurrentContext, _ = kubectl("config", "current-context")
	a.logInfo(fmt.Sprintf("Context: %s", a.CurrentContext))

	a.logStep("Counting cluster nodes...")
	nodeOut, _ := kubectl("get", "nodes", "--no-headers")
	lines := nonEmpty(strings.Split(nodeOut, "\n"))
	a.NodeCount = len(lines)
	for _, l := range lines {
		if strings.Contains(l, "Ready") {
			a.ReadyNodes++
		}
	}
	a.logInfo(fmt.Sprintf("Cluster nodes: %d/%d ready", a.ReadyNodes, a.NodeCount))

	if a.NodeCount > 0 {
		a.writeln("\n  Node details:")
		nodeDetails, _ := kubectl("get", "nodes",
			"-o", "custom-columns=NAME:.metadata.name,STATUS:.status.conditions[-1].type,VERSION:.status.nodeInfo.kubeletVersion",
			"--no-headers")
		for _, l := range strings.Split(nodeDetails, "\n") {
			if strings.TrimSpace(l) != "" {
				a.writeln(fmt.Sprintf("    %s", l))
			}
		}
	}

	// ── Namespace ────────────────────────────────────
	a.printSection("Namespace Validation")
	a.logStep(fmt.Sprintf("Executing: kubectl get namespace %s...", a.Namespace))

	if _, _, err := kubectlE("get", "namespace", a.Namespace); err != nil {
		a.logFail(fmt.Sprintf("Namespace '%s' not found", a.Namespace))
		ns, _ := kubectl("get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
		a.logInfo(fmt.Sprintf("Available: %s", ns))
		os.Exit(1)
	}
	a.logPass(fmt.Sprintf("Namespace '%s' exists", a.Namespace))

	nsStatus, _ := kubectl("get", "namespace", a.Namespace, "-o", "jsonpath={.status.phase}")
	nsAge, _ := kubectl("get", "namespace", a.Namespace, "-o", "jsonpath={.metadata.creationTimestamp}")
	a.logInfo(fmt.Sprintf("Status: %s", nsStatus))
	a.logInfo(fmt.Sprintf("Created: %s", nsAge))

	a.logStep("Counting namespace resources...")
	podOut, _ := kubectl("get", "pods", "-n", a.Namespace, "--no-headers")
	svcOut, _ := kubectl("get", "svc", "-n", a.Namespace, "--no-headers")
	cmOut, _ := kubectl("get", "cm", "-n", a.Namespace, "--no-headers")
	pc := len(nonEmpty(strings.Split(podOut, "\n")))
	sc := len(nonEmpty(strings.Split(svcOut, "\n")))
	cc := len(nonEmpty(strings.Split(cmOut, "\n")))
	a.logInfo(fmt.Sprintf("Resources: %d pods, %d services, %d configmaps", pc, sc, cc))

	if pc > 0 {
		a.writeln("\n  Pod summary:")
		podSum, _ := kubectl("get", "pods", "-n", a.Namespace,
			"-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,RESTARTS:.status.containerStatuses[0].restartCount",
			"--no-headers")
		for _, l := range strings.Split(podSum, "\n") {
			if strings.TrimSpace(l) != "" {
				a.writeln(fmt.Sprintf("    %s", l))
			}
		}
	}

	// ── RBAC ─────────────────────────────────────────
	a.printSection("RBAC Permissions Check")

	type rbacCheck struct{ verb, resource, extra string }
	rbacChecks := []rbacCheck{
		{"get", "pods", "--all-namespaces"},
		{"get", "services", "--all-namespaces"},
		{"get", "validatingwebhookconfigurations", ""},
		{"get", "networkpolicies", "--all-namespaces"},
	}
	for _, c := range rbacChecks {
		args := []string{"auth", "can-i", c.verb, c.resource}
		if c.extra != "" {
			args = append(args, c.extra)
		}
		a.logStep(fmt.Sprintf("Testing 'kubectl %s'...", strings.Join(args[2:], " ")))
		out, err := kubectl(args...)
		if err == nil && strings.TrimSpace(out) == "yes" {
			a.logPass(fmt.Sprintf("Can %s %s", c.verb, c.resource))
		} else {
			a.logWarn(fmt.Sprintf("Limited permission: %s %s", c.verb, c.resource))
		}
	}

	a.logInfo("Pre-flight checks completed ✓")
}

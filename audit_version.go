package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
// PHASE 2 — Version Audit
// ─────────────────────────────────────────────

func (a *AuditState) auditVersion() {
	a.printHeader("PHASE 2 — VERSION AUDIT")

	// ── Discover controller ──────────────────────────
	a.printSection("Controller Deployment Discovery")
	a.logStep("Checking for DaemonSet deployment...")

	if _, err := kubectl("get", "daemonset", "-n", a.Namespace, a.ControllerName); err == nil {
		a.DeploymentType = "DaemonSet"
		a.logPass("Found controller as DaemonSet")
		a.logStep("Fetching container image...")
		a.ControllerImage, _ = kubectl("get", "daemonset", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.spec.template.spec.containers[0].image}")
		a.logStep("Checking replica status...")
		ready, _ := kubectl("get", "daemonset", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.status.numberReady}")
		desired, _ := kubectl("get", "daemonset", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.status.desiredNumberScheduled}")
		a.ControllerReplicas = ready + "/" + desired
	} else {
		a.logStep("Not a DaemonSet, checking for Deployment...")
		if _, err := kubectl("get", "deployment", "-n", a.Namespace, a.ControllerName); err == nil {
			a.DeploymentType = "Deployment"
			a.logPass("Found controller as Deployment")
			a.logStep("Fetching container image...")
			a.ControllerImage, _ = kubectl("get", "deployment", "-n", a.Namespace,
				a.ControllerName, "-o", "jsonpath={.spec.template.spec.containers[0].image}")
			a.logStep("Checking replica status...")
			readyRep, _ := kubectl("get", "deployment", "-n", a.Namespace,
				a.ControllerName, "-o", "jsonpath={.status.readyReplicas}")
			desiredRep, _ := kubectl("get", "deployment", "-n", a.Namespace,
				a.ControllerName, "-o", "jsonpath={.spec.replicas}")
			a.ControllerReplicas = readyRep + "/" + desiredRep
		} else {
			a.logFail("No ingress-nginx-controller found (neither DaemonSet nor Deployment)")
			a.logStep("Searching for alternative controller names...")
			out, _ := kubectl("get", "deployments,daemonsets", "-n", a.Namespace, "-o", "name")
			a.writeln(out)
			return
		}
	}

	a.logInfo(fmt.Sprintf("Ready replicas: %s", a.ControllerReplicas))

	// ── Version analysis ─────────────────────────────
	a.printSection("Version Analysis")
	a.logInfo(fmt.Sprintf("Deployment type:  %s", a.DeploymentType))
	a.logInfo(fmt.Sprintf("Container image:  %s", a.ControllerImage))

	a.logStep("Extracting version from image tag...")
	a.ControllerVersion = extractVersion(a.ControllerImage)
	a.logInfo(fmt.Sprintf("Controller version: %s", a.ControllerVersion))
	a.logInfo(fmt.Sprintf("Image registry: %s", strings.Split(a.ControllerImage, "/")[0]))

	if strings.Contains(a.ControllerImage, "@sha256:") {
		parts := strings.SplitN(a.ControllerImage, "@", 2)
		a.logInfo(fmt.Sprintf("Image SHA: %s...", truncate(parts[1], 20)))
	}

	const latestVersion = "v1.14.3"
	a.logStep(fmt.Sprintf("Comparing %s with latest %s...", a.ControllerVersion, latestVersion))

	switch {
	case a.ControllerVersion == latestVersion:
		a.logPass(fmt.Sprintf("Running latest stable version %s ✓", latestVersion))
	case a.ControllerVersion == "unknown":
		a.logWarn("Could not determine version from image tag — manual verification required")
	default:
		a.logFail(fmt.Sprintf("Outdated version %s (latest: %s)", a.ControllerVersion, latestVersion))
		a.logInfo(fmt.Sprintf(
			"Upgrade command: helm upgrade ingress-nginx ingress-nginx/ingress-nginx --version 4.14.3 -n %s",
			a.Namespace))
		ns := a.Namespace
		a.addFix("upgrade-controller", "CRITICAL",
			fmt.Sprintf("Upgrade ingress-nginx controller from %s to v1.14.3", a.ControllerVersion),
			fmt.Sprintf("helm upgrade ingress-nginx ingress-nginx/ingress-nginx --version 4.14.3 -n %s", ns),
			func() error {
				return runCmd("helm", "upgrade", "ingress-nginx",
					"ingress-nginx/ingress-nginx", "--version", "4.14.3", "-n", ns)
			})
	}

	// ── Helm chart ──────────────────────────────────
	a.printSection("Helm Chart Information")
	a.logStep(fmt.Sprintf("Querying Helm releases in namespace %s...", a.Namespace))

	helmJSON, err := helmCmd("list", "-n", a.Namespace, "-o", "json")
	if err == nil && helmJSON != "" {
		var releases []map[string]interface{}
		if json.Unmarshal([]byte(helmJSON), &releases) == nil {
			for _, r := range releases {
				if r["name"] == "ingress-nginx" {
					a.HelmChart = fmt.Sprintf("%v", r["chart"])
					a.HelmStatus = fmt.Sprintf("%v", r["status"])
					a.HelmRevision = fmt.Sprintf("%v", r["revision"])
					a.HelmChartVersion = extractVersion(a.HelmChart)
					a.logInfo(fmt.Sprintf("Helm chart:    %s", a.HelmChart))
					a.logInfo(fmt.Sprintf("Status:        %s", a.HelmStatus))
					a.logInfo(fmt.Sprintf("Revision:      %s", a.HelmRevision))
					if a.HelmChartVersion == "4.14.3" {
						a.logPass("Running latest Helm chart version 4.14.3")
					} else {
						a.logWarn(fmt.Sprintf("Chart version %s may be outdated (latest: 4.14.3)", a.HelmChartVersion))
					}
				}
			}
		}
	} else {
		a.logWarn("Could not query Helm releases — not installed via Helm or Helm not available")
	}

	// ── Update config ────────────────────────────────
	a.printSection("Update Configuration")
	resType := strings.ToLower(a.DeploymentType)

	a.logStep("Fetching image pull policy...")
	a.ImagePullPolicy, _ = kubectl("get", resType, "-n", a.Namespace,
		a.ControllerName, "-o", "jsonpath={.spec.template.spec.containers[0].imagePullPolicy}")
	a.logInfo(fmt.Sprintf("Image pull policy: %s", a.ImagePullPolicy))

	a.logStep("Fetching update strategy...")
	if a.DeploymentType == "DaemonSet" {
		a.UpdateStrategy, _ = kubectl("get", "daemonset", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.spec.updateStrategy.type}")
	} else {
		a.UpdateStrategy, _ = kubectl("get", "deployment", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.spec.strategy.type}")
		surge, _ := kubectl("get", "deployment", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.spec.strategy.rollingUpdate.maxSurge}")
		unavail, _ := kubectl("get", "deployment", "-n", a.Namespace,
			a.ControllerName, "-o", "jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}")
		a.logInfo(fmt.Sprintf("Update strategy: %s (maxSurge: %s, maxUnavailable: %s)",
			a.UpdateStrategy, surge, unavail))
	}
	a.logInfo(fmt.Sprintf("Update strategy: %s", a.UpdateStrategy))

	// ── Lifecycle warning ────────────────────────────
	a.printSection("Project Lifecycle Status")
	a.logWarn("⚠️  IMPORTANT: Ingress-NGINX community project is retiring in March 2026")
	a.logInfo("Timeline: ~1 month remaining before end-of-life")
	a.logInfo("Impact: No security updates, bug fixes, or support after March 2026")
	a.logInfo("Action required: Plan migration to Gateway API or alternative controller")
	a.writeln("\n  Recommended alternatives:")
	a.writeln("    1. Gateway API (recommended) — Future-proof Kubernetes standard")
	a.writeln("    2. Traefik            — Drop-in replacement, actively maintained")
	a.writeln("    3. F5 NGINX Ingress   — Commercial, actively maintained")
	a.writeln("    4. Istio              — Service mesh with ingress capabilities")
}

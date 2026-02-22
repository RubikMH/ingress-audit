package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
// PHASE 5 — Configuration Audit
// ─────────────────────────────────────────────

func (a *AuditState) auditConfiguration() {
	a.printHeader("PHASE 5 — CONFIGURATION AUDIT")

	// ── ConfigMap settings ───────────────────────────
	a.printSection("ConfigMap Settings")
	a.logStep("Fetching ingress-nginx-controller configmap...")

	cmJSON, err := kubectl("get", "configmap", "-n", a.Namespace, a.ControllerName, "-o", "json")
	if err == nil {
		var cm map[string]interface{}
		if json.Unmarshal([]byte(cmJSON), &cm) == nil {
			data, _ := cm["data"].(map[string]interface{})
			if data == nil {
				data = map[string]interface{}{}
			}

			a.logStep("Checking allow-snippet-annotations...")
			a.AllowSnippets = fmt.Sprintf("%v", data["allow-snippet-annotations"])
			if a.AllowSnippets == "true" {
				a.logFail("SECURITY RISK: allow-snippet-annotations is enabled")
				a.logInfo("REMEDIATION: Disable snippet annotations unless absolutely required")
				ns := a.Namespace
				a.addFix("snippet-annotations", "CRITICAL",
					"Disable allow-snippet-annotations in ingress-nginx ConfigMap",
					fmt.Sprintf(`kubectl patch cm ingress-nginx-controller -n %s --type merge -p '{"data":{"allow-snippet-annotations":"false"}}'`, ns),
					func() error {
						return runCmd("kubectl", "patch", "cm", a.ControllerName,
							"-n", ns, "--type", "merge",
							"-p", `{"data":{"allow-snippet-annotations":"false"}}`)
					})
			} else {
				a.logPass("Snippet annotations disabled (secure default)")
			}

			a.logStep("Checking SSL protocols...")
			sslProto := fmt.Sprintf("%v", data["ssl-protocols"])
			a.logInfo(fmt.Sprintf("SSL protocols: %s", sslProto))
			if strings.Contains(sslProto, "TLSv1 ") {
				a.logWarn("TLSv1 is enabled — consider disabling for better security")
			}

			a.logStep("Checking custom HTTP errors...")
			customErr := fmt.Sprintf("%v", data["custom-http-errors"])
			a.logInfo(fmt.Sprintf("Custom HTTP errors: %s", customErr))
		}
	} else {
		a.logWarn("ConfigMap 'ingress-nginx-controller' not found")
	}

	// ── Resource limits ──────────────────────────────
	a.printSection("Resource Limits")
	resType := strings.ToLower(a.DeploymentType)
	if resType == "" {
		resType = "deployment"
	}

	a.logStep("Fetching resource limits...")
	a.CPURequest, _ = kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.containers[0].resources.requests.cpu}")
	a.CPULimit, _ = kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.containers[0].resources.limits.cpu}")
	a.MemoryRequest, _ = kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.containers[0].resources.requests.memory}")
	a.MemoryLimit, _ = kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.containers[0].resources.limits.memory}")

	a.logInfo(fmt.Sprintf("CPU:    request=%s  limit=%s", orDefault(a.CPURequest), orDefault(a.CPULimit)))
	a.logInfo(fmt.Sprintf("Memory: request=%s  limit=%s", orDefault(a.MemoryRequest), orDefault(a.MemoryLimit)))

	if a.CPULimit == "" || a.MemoryLimit == "" {
		if a.DeploymentType != "" {
			a.logWarn("Resource limits not set — may impact cluster stability")
			ns := a.Namespace
			rt := strings.ToLower(a.DeploymentType)
			if rt == "" {
				rt = "deployment"
			}
			a.addFix("resource-limits", "WARNING",
				"Set default resource limits on ingress-nginx-controller",
				fmt.Sprintf("kubectl set resources %s ingress-nginx-controller -n %s --limits=cpu=200m,memory=256Mi --requests=cpu=100m,memory=128Mi", rt, ns),
				func() error {
					return runCmd("kubectl", "set", "resources", rt, a.ControllerName,
						"-n", ns,
						"--limits=cpu=200m,memory=256Mi",
						"--requests=cpu=100m,memory=128Mi")
				})
		}
	} else {
		a.logPass("Resource limits configured")
	}
}

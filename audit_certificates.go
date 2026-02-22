package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ─────────────────────────────────────────────
// PHASE 8 — TLS/SSL Certificate Audit
// ─────────────────────────────────────────────

func (a *AuditState) auditCertificates() {
	a.printHeader("PHASE 8 — TLS/SSL CERTIFICATE AUDIT")

	// ── Admission webhook cert ───────────────────────
	a.printSection("Admission Webhook Certificates")
	a.logStep("Checking admission webhook certificate secret...")

	if _, err := kubectl("get", "secret", "-n", a.Namespace, "ingress-nginx-admission"); err != nil {
		a.logWarn("Admission webhook certificate secret not found")
	} else {
		a.logPass("Admission webhook certificate secret exists")

		a.logStep("Decoding certificate...")
		certB64, _ := kubectl("get", "secret", "-n", a.Namespace,
			"ingress-nginx-admission", "-o", "jsonpath={.data.cert}")
		if certB64 != "" {
			certPEM, err := base64.StdEncoding.DecodeString(certB64)
			if err == nil {
				cmd := exec.Command("openssl", "x509", "-noout", "-enddate")
				cmd.Stdin = bytes.NewReader(certPEM)
				out, err := cmd.Output()
				if err == nil {
					expiry := strings.TrimPrefix(strings.TrimSpace(string(out)), "notAfter=")
					a.logInfo(fmt.Sprintf("Certificate expiry: %s", expiry))
					expiryTime, parseErr := time.Parse("Jan  2 15:04:05 2006 MST", expiry)
					if parseErr == nil {
						daysLeft := int(time.Until(expiryTime).Hours() / 24)
						switch {
						case daysLeft < 0:
							a.logFail("Certificate has EXPIRED!")
						case daysLeft < 30:
							a.logWarn(fmt.Sprintf("Certificate expires in %d days — renewal needed soon", daysLeft))
						default:
							a.logPass(fmt.Sprintf("Certificate valid for %d days", daysLeft))
						}
					}
				}
			}
		}
	}

	// ── Default SSL certificate ──────────────────────
	a.printSection("Default SSL Certificate")
	resType := strings.ToLower(a.DeploymentType)
	if resType == "" {
		resType = "deployment"
	}

	a.logStep("Checking default SSL certificate arg...")
	args, _ := kubectl("get", resType, "-n", a.Namespace, a.ControllerName,
		"-o", "jsonpath={.spec.template.spec.containers[0].args}")

	defaultCert := "not-set"
	for _, arg := range strings.Fields(args) {
		if strings.HasPrefix(arg, "--default-ssl-certificate=") {
			defaultCert = strings.TrimPrefix(arg, "--default-ssl-certificate=")
		}
	}

	if defaultCert != "not-set" {
		a.logInfo(fmt.Sprintf("Default SSL certificate: %s", defaultCert))
		parts := strings.SplitN(defaultCert, "/", 2)
		if len(parts) == 2 {
			if _, err := kubectl("get", "secret", "-n", parts[0], parts[1]); err == nil {
				a.logPass("Default SSL certificate secret exists")
			} else {
				a.logFail(fmt.Sprintf("Default SSL certificate secret '%s' not found", defaultCert))
			}
		}
	} else {
		a.logInfo("No default SSL certificate configured (will use self-signed)")
	}
}

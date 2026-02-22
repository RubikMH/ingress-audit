package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PHASE 9 -- Ingress Resources Audit

func (a *AuditState) auditIngressResources() {
	a.printHeader("PHASE 9 — INGRESS RESOURCES AUDIT")

	// -- Cluster-wide inventory
	a.printSection("Cluster-wide Ingress Resources")
	a.logStep("Listing all Ingress resources...")

	ingressOut, _ := kubectl("get", "ingress", "-A", "--no-headers")
	totalIngress := len(nonEmpty(strings.Split(ingressOut, "\n")))
	a.logInfo(fmt.Sprintf("Total Ingress resources: %d", totalIngress))

	a.logStep("Counting NGINX-class Ingress resources...")
	ingressJSON, _ := kubectl("get", "ingress", "-A", "-o", "json")
	nginxCount := 0
	snippetNames := []string{}
	tlsCount := 0

	if ingressJSON != "" {
		var list map[string]interface{}
		if json.Unmarshal([]byte(ingressJSON), &list) == nil {
			items, _ := list["items"].([]interface{})
			for _, item := range items {
				im, _ := item.(map[string]interface{})
				meta := getMap(im, "metadata")
				spec := getMap(im, "spec")
				anns, _ := meta["annotations"].(map[string]interface{})
				ingressClass := fmt.Sprintf("%v", spec["ingressClassName"])
				annClass := fmt.Sprintf("%v", anns["kubernetes.io/ingress.class"])
				if ingressClass == "nginx" || annClass == "nginx" {
					nginxCount++
					ns := fmt.Sprintf("%v", meta["namespace"])
					name := fmt.Sprintf("%v", meta["name"])
					// check snippets
					for k := range anns {
						if strings.Contains(k, "snippet") {
							snippetNames = append(snippetNames, ns+"/"+name)
						}
					}
					// check TLS
					if _, hasTLS := spec["tls"]; hasTLS {
						tlsCount++
					}
				}
			}
		}
	}

	a.logInfo(fmt.Sprintf("NGINX Ingress resources: %d", nginxCount))
	if nginxCount > 0 {
		a.logPass(fmt.Sprintf("Found %d Ingress resources using nginx class", nginxCount))
	}

	// -- Snippet annotation check
	a.printSection("Snippet Annotation Check")
	a.logStep("Scanning for snippet annotations...")
	if len(snippetNames) > 0 {
		a.logWarn(fmt.Sprintf("Found %d Ingress resources using snippet annotations", len(snippetNames)))
		a.logInfo("Snippets can be a security risk — review carefully")
		for _, n := range snippetNames {
			a.writeln(fmt.Sprintf("    - %s", n))
		}
	} else {
		a.logPass("No Ingress resources using snippet annotations")
	}

	// -- TLS coverage
	a.printSection("TLS Configuration")
	a.logStep("Checking TLS coverage...")
	a.logInfo(fmt.Sprintf("Ingress with TLS: %d/%d", tlsCount, nginxCount))
	if nginxCount > 0 && tlsCount < nginxCount {
		a.logWarn(fmt.Sprintf("%d Ingress resources without TLS configured for %s",
			nginxCount-tlsCount, a.Domain))
	} else if nginxCount > 0 {
		a.logPass("All Ingress resources configured with TLS")
	}
}

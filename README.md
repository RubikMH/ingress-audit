# ingress-audit

A terminal-based security auditor for **ingress-nginx** controllers running on Kubernetes. It runs nine audit phases, prints a colour-coded report, optionally applies fixes, and writes both a plain-text and a structured JSON report to disk.

---

## Requirements

| Tool | Purpose |
|------|---------|
| `kubectl` | Query the cluster (must be configured and authenticated) |
| `helm` *(optional)* | Detect Helm chart version / release status |
| `openssl` *(optional)* | Parse TLS certificate expiry dates |
| Go 1.21+ | Only needed if building from source |

`kubectl` must have access to the target cluster before you run the tool.

---

## Installation

### Build from source

```bash
git clone https://github.com/RubikMH/ingress-audit.git
cd ingress-audit
go build -o ingress-audit .
```

### Run directly without installing

```bash
go run .
```

---

## Usage

```bash
./ingress-audit
```

The tool is fully interactive — no flags required. On launch it:

1. Clears the screen and shows the ASCII banner.
2. Prompts for your **domain**, **admin email**, and **controller resource name**.
3. Scans the cluster for namespaces that contain an ingress-nginx controller and lets you pick one or all.
4. Runs all nine audit phases and prints results live.
5. Writes two report files to the current directory.
6. Offers to apply any auto-fixable issues (press `y` to apply, `n` to skip each one).

### Interactive prompts

```
  Domain (e.g. rubikmh.io) [default: rubikmh.io]:
  Admin email [default: admin@rubikmh.io]:
  Ingress controller resource name [default: ingress-nginx-controller]:
```

Press **Enter** to accept the default value shown in brackets.

### Namespace selection

```
  #     NAMESPACE                       INGRESS-NGINX
  ────────────────────────────────────────────────────
  [ 0]  ✦ Scan ALL of the above
  ────────────────────────────────────────────────────
  [ 1]  ingress-nginx                   found
  [ 2]  production                      found

  Enter number (or comma-separated list, 0 = all): 1
```

- Enter a **single number** to audit one namespace.
- Enter a **comma-separated list** (e.g. `1,2`) to audit several namespaces.
- Enter `0` or `all` to scan every namespace that has an ingress-nginx controller.

---

## Audit Phases

| Phase | Name | What it checks |
|-------|------|----------------|
| 1 | Preflight | `kubectl` connectivity, API server, nodes, current context |
| 2 | Version | Controller image version, Helm chart, latest vs installed |
| 3 | Admission Controller | Service type (ClusterIP vs exposed), AbuseBSI report compliance |
| 4 | Network Security | NetworkPolicies attached to the controller |
| 5 | Configuration | `allow-snippet-annotations`, resource limits, image pull policy |
| 6 | Pod Security | Update strategy, security context, `runAsNonRoot` |
| 7 | Vulnerabilities | CVE status for current version, AbuseBSI CB-Report#20260218-10009947 |
| 8 | Certificates | TLS cert expiry dates via ServiceAccount token / openssl |
| 9 | Ingress Resources | NGINX-class Ingress count, snippet annotations, TLS coverage |

---

## Output Files

After each run the tool writes two files in the **current working directory**:

| File | Description |
|------|-------------|
| `ingress-audit-<timestamp>.txt` | Full plain-text report (ANSI codes stripped) |
| `ingress-audit-<timestamp>.json` | Structured JSON report (see schema below) |

When scanning multiple namespaces each namespace gets its own pair of files:

```
ingress-audit-ingress-nginx-20260222-143012.txt
ingress-audit-ingress-nginx-20260222-143012.json
ingress-audit-production-20260222-143012.txt
ingress-audit-production-20260222-143012.json
```

### JSON report schema

```json
{
  "audit_timestamp": "2026-02-22T14:30:12Z",
  "domain": "rubikmh.io",
  "admin_email": "admin@rubikmh.io",
  "cluster_version": "v1.29.0",
  "namespace": "ingress-nginx",
  "controller": {
    "deployment_type": "Deployment",
    "version": "v1.11.0",
    "image": "registry.k8s.io/ingress-nginx/controller:v1.11.0",
    "latest_version": "v1.14.3"
  },
  "admission_controller": {
    "service_type": "ClusterIP",
    "cluster_ip": "10.96.0.1",
    "external_ip": "",
    "publicly_exposed": false
  },
  "security": {
    "abusebsi_compliant": true,
    "snippet_annotations_enabled": false,
    "network_policies_count": 1
  },
  "audit_results": {
    "passed": 18,
    "failed": 2,
    "warnings": 3,
    "info": 7
  },
  "recommendations": [
    "Upgrade controller to v1.14.3",
    "Plan migration from Ingress-NGINX (retiring March 2026)",
    "Consider migrating to Gateway API or alternative controller"
  ]
}
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All checks passed (or only warnings/info) |
| `1` | One or more **FAIL** findings |

This makes the tool suitable for use in CI pipelines:

```bash
./ingress-audit || echo "Audit failed — review findings above"
```

---

## Auto-fixes

When fixable issues are detected the tool prompts at the end:

```
  [FIX-001]  CRITICAL: Change admission controller service to ClusterIP
  Command: kubectl patch svc ingress-nginx-controller-admission ...
  Apply this fix? [y/N]:
```

Fixes are applied one at a time and each result is shown immediately. You can skip any fix by pressing **Enter** or typing `n`.

---

## Running Tests

```bash
go test ./...
```

To see verbose output per test:

```bash
go test -v ./...
```

Test coverage spans `util.go` (pure functions), `shell.go` (binary detection, ANSI stripping), `state.go` (counters, output buffer, fix registration), and `report.go` (recommendation logic).

---

## Project Structure

```
ingress-audit/
├── main.go                   # Entry point, runAudit, runMultiNamespaceScan
├── colors.go                 # ANSI color constants
├── ui.go                     # Lipgloss box renderer
├── shell.go                  # kubectl / helm / cmdExists / stripANSI
├── util.go                   # Pure helper functions
├── state.go                  # AuditState struct, logging, counters
├── setup.go                  # Interactive setup & namespace picker
├── audit_preflight.go        # Phase 1
├── audit_version.go          # Phase 2
├── audit_admission.go        # Phase 3
├── audit_network.go          # Phase 4
├── audit_config.go           # Phase 5
├── audit_podsecurity.go      # Phase 6
├── audit_vulnerabilities.go  # Phase 7
├── audit_certificates.go     # Phase 8
├── audit_ingress.go          # Phase 9
├── report.go                 # JSON report structs & summary
├── fix.go                    # Fix execution engine
├── util_test.go
├── shell_test.go
├── state_test.go
└── report_test.go
```

---

## Notes

- The tool requires a **working kubeconfig** (`~/.kube/config` or `KUBECONFIG` env var).
- It only reads from the cluster — no writes happen unless you explicitly approve a fix.
- The admission controller exposure check directly relates to **AbuseBSI CB-Report#20260218-10009947**.
- ingress-nginx is scheduled for retirement in **March 2026**. The tool always includes migration reminders in its recommendations.

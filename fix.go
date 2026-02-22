package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// runCmd is the low-level executor used by all fix functions.
// It streams stdout/stderr in real-time so the user sees progress.
// ─────────────────────────────────────────────────────────────────────────────

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ─────────────────────────────────────────────────────────────────────────────
// fixDeleteExposingIngress deletes every Ingress resource (ns/name) that was
// found exposing the admission controller endpoint.
// ─────────────────────────────────────────────────────────────────────────────

func fixDeleteExposingIngress(exposing string) error {
	lines := strings.Split(strings.TrimSpace(exposing), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) != 2 {
			fmt.Printf("  %s⚠ Skipping unrecognised entry: %s%s\n", Yellow, line, Reset)
			continue
		}
		ns, name := parts[0], parts[1]

		fmt.Printf("  %s→%s Deleting Ingress %s/%s ...\n", Cyan, Reset, ns, name)
		if err := runCmd("kubectl", "delete", "ingress", name, "-n", ns); err != nil {
			return fmt.Errorf("delete ingress %s/%s: %w", ns, name, err)
		}
		fmt.Printf("  %s✓%s Deleted %s/%s\n", Green, Reset, ns, name)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// waitForRollout blocks until the Deployment/DaemonSet finishes rolling out
// after a fix is applied, so the user sees the cluster settle.
// ─────────────────────────────────────────────────────────────────────────────

func waitForRollout(resourceType, name, ns string) {
	fmt.Printf("  %s→%s Waiting for rollout of %s/%s in %s ...\n",
		Cyan, Reset, resourceType, name, ns)
	_ = runCmd("kubectl", "rollout", "status",
		resourceType+"/"+name, "-n", ns, "--timeout=120s")
}

// ─────────────────────────────────────────────────────────────────────────────
// offerFixes is called at the end of the audit when fixable issues exist.
// It shows a numbered checklist, asks yes/no, then runs the fixes one by one.
// ─────────────────────────────────────────────────────────────────────────────

func offerFixes(a *AuditState) {
	fmt.Printf("\n%s%s AUTO-FIX AVAILABLE %s%s\n",
		Bold+Yellow,
		strings.Repeat("═", 10),
		strings.Repeat("═", 10),
		Reset)

	fmt.Printf("\n  The audit found %s%d fixable issue(s)%s for namespace %s%s%s:\n\n",
		Bold, len(a.Fixes), Reset,
		Cyan, a.Namespace, Reset)

	// Print fix table
	fmt.Printf("  %-4s  %-10s  %s\n", "#", "SEVERITY", "DESCRIPTION")
	fmt.Printf("  %s\n", strings.Repeat("─", 70))

	for i, fix := range a.Fixes {
		severityColor := Yellow
		if fix.Severity == "CRITICAL" {
			severityColor = Red
		}
		fmt.Printf("  %s[%2d]%s  %s%-10s%s  %s\n",
			Bold, i+1, Reset,
			severityColor, fix.Severity, Reset,
			fix.Description)
		fmt.Printf("        %s$ %s%s\n", Dim, fix.Command, Reset)
		fmt.Println()
	}

	fmt.Printf("  %s\n", strings.Repeat("─", 70))
	fmt.Printf("\n  %s⚠  These changes will be applied to your live cluster.%s\n", Yellow+Bold, Reset)
	fmt.Printf("  %s   Review the commands above before continuing.%s\n\n", Yellow, Reset)

	// Ask user
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("  %sDo you want to apply all fixes? [yes/no]:%s ", Bold, Reset)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))

	if answer != "yes" && answer != "y" {
		fmt.Printf("\n  %sSkipping auto-fix.%s Fixes were logged in %s\n",
			Yellow, Reset, a.TextReportFile)
		fmt.Println("  You can apply them manually using the commands shown above.")
		return
	}

	// ── Run each fix ──────────────────────────────────────────────────────
	fmt.Printf("\n%s%s APPLYING FIXES %s%s\n",
		Bold+Blue,
		strings.Repeat("═", 10),
		strings.Repeat("═", 10),
		Reset)

	results := make([]fixResult, 0, len(a.Fixes))

	for i, fix := range a.Fixes {
		fmt.Printf("\n  %s[%d/%d]%s %s%s%s\n",
			Bold+Cyan, i+1, len(a.Fixes), Reset,
			Bold, fix.Description, Reset)
		fmt.Printf("  %s$ %s%s\n\n", Dim, fix.Command, Reset)

		start := time.Now()
		err := fix.Run()
		elapsed := time.Since(start).Round(time.Millisecond)

		if err != nil {
			fmt.Printf("\n  %s✗ FAILED%s (%v): %v\n", Red+Bold, Reset, elapsed, err)
			results = append(results, fixResult{fix: fix, err: err})
		} else {
			fmt.Printf("\n  %s✓ DONE%s (%v)\n", Green+Bold, Reset, elapsed)
			results = append(results, fixResult{fix: fix, err: nil})

			// For changes that affect running pods, wait for rollout
			switch fix.ID {
			case "snippet-annotations", "resource-limits":
				resType := strings.ToLower(a.DeploymentType)
				if resType == "" {
					resType = "deployment"
				}
				waitForRollout(resType, "ingress-nginx-controller", a.Namespace)
			case "upgrade-controller":
				waitForRollout("deployment", "ingress-nginx-controller", a.Namespace)
			}
		}
	}

	// ── Fix summary ───────────────────────────────────────────────────────
	fmt.Printf("\n%s%s FIX SUMMARY %s%s\n",
		Bold+Blue,
		strings.Repeat("═", 10),
		strings.Repeat("═", 10),
		Reset)

	passed, failed := 0, 0
	for _, r := range results {
		if r.err == nil {
			fmt.Printf("  %s✓%s  %s\n", Green, Reset, r.fix.Description)
			passed++
		} else {
			fmt.Printf("  %s✗%s  %s\n      %sError: %v%s\n",
				Red, Reset, r.fix.Description,
				Dim, r.err, Reset)
			failed++
		}
	}

	fmt.Println()
	if failed == 0 {
		fmt.Printf("  %s%s✓ All %d fix(es) applied successfully.%s\n",
			Green, Bold, passed, Reset)
		fmt.Printf("  %sRe-run the audit to confirm all issues are resolved.%s\n\n", Dim, Reset)
	} else {
		fmt.Printf("  %s%s✗ %d fix(es) failed, %d succeeded.%s\n",
			Red, Bold, failed, passed, Reset)
		fmt.Printf("  %sApply the failed fixes manually using the commands shown above.%s\n\n",
			Yellow, Reset)
	}
}

// fixResult holds the outcome of a single fix execution.
type fixResult struct {
	fix Fix
	err error
}

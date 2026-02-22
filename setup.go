package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/motki/cli/text/banner"
)

// ----- Interactive Setup -----

// promptUser prints a prompt and reads a line from stdin.
// If the user presses Enter without typing anything, defaultVal is returned.
func promptUser(prompt, defaultVal string) string {
	reader := bufio.NewReader(os.Stdin)
	if defaultVal != "" {
		fmt.Printf("  %s%s%s [default: %s%s%s]: ", Bold, prompt, Reset, Cyan, defaultVal, Reset)
	} else {
		fmt.Printf("  %s%s%s: ", Bold, prompt, Reset)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

// ----- Namespace discovery & picker -----

// fetchNamespaces returns all namespace names visible to kubectl.
func fetchNamespaces() ([]string, error) {
	out, err := exec.Command("kubectl", "get", "namespaces",
		"-o", "jsonpath={.items[*].metadata.name}").Output()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, fmt.Errorf("no namespaces returned")
	}
	return strings.Fields(raw), nil
}

// nsHasIngressNginx returns true when the given namespace contains a
// Deployment or DaemonSet whose name matches controllerName.
func nsHasIngressNginx(ns, controllerName string) bool {
	for _, resource := range []string{"deployment", "daemonset"} {
		out, _ := exec.Command("kubectl", "get", resource, "-n", ns,
			controllerName, "--ignore-not-found").Output()
		if strings.TrimSpace(string(out)) != "" {
			return true
		}
	}
	return false
}

// pickNamespace presents an interactive list of namespaces that contain an
// ingress-nginx controller and stores the user's selection in a.
func pickNamespace(a *AuditState) {
	fmt.Printf("\n%s\U0001F4E6 Namespace Selection%s\n", Bold+Blue, Reset)
	fmt.Println(strings.Repeat("\u2500", 55))
	fmt.Printf("  %sFetching namespaces from cluster...%s", Dim, Reset)
	allNamespaces, err := fetchNamespaces()
	if err != nil {
		fmt.Printf(" %sFAILED%s\n", Red, Reset)
		fmt.Printf("  %sCannot reach cluster. Using manual input.%s\n", Yellow, Reset)
		a.Namespace = promptUser("Namespace", "ingress-nginx")
		a.Namespaces = []string{a.Namespace}
		return
	}
	fmt.Printf(" %s%d total%s\n", Green, len(allNamespaces), Reset)
	var nginxNamespaces []string
	fmt.Printf("  %sScanning for ingress-nginx controller...%s\n", Dim, Reset)
	for _, ns := range allNamespaces {
		if nsHasIngressNginx(ns, a.ControllerName) {
			nginxNamespaces = append(nginxNamespaces, ns)
		}
	}
	if len(nginxNamespaces) == 0 {
		fmt.Printf("\n  %sNo ingress-nginx controller found in any namespace.%s\n", Red, Reset)
		fmt.Printf("  %sEnter namespace manually or press Enter to exit:%s ", Yellow, Reset)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			os.Exit(0)
		}
		a.Namespace = input
		a.Namespaces = []string{input}
		return
	}
	fmt.Println()
	fmt.Printf("  %s%-4s  %-30s  %s%s\n", Bold, "#", "NAMESPACE", "INGRESS-NGINX", Reset)
	fmt.Printf("  %s\n", strings.Repeat("\u2500", 50))
	if len(nginxNamespaces) > 1 {
		fmt.Printf("  %s%-4s  %-30s%s\n", Cyan+Bold, "[ 0]", "Scan ALL of the above", Reset)
		fmt.Printf("  %s\n", strings.Repeat("\u2500", 50))
	}
	for i, ns := range nginxNamespaces {
		fmt.Printf("  %s[%2d]%s  %s%-30s%s  %sfound%s\n",
			Bold, i+1, Reset,
			Cyan, ns, Reset,
			Green, Reset)
	}
	fmt.Println()
	if len(nginxNamespaces) == 1 {
		a.Namespace = nginxNamespaces[0]
		a.Namespaces = nginxNamespaces
		fmt.Printf("  %sAuto-selected: %s%s%s (only controller found)\n",
			Green, Cyan, nginxNamespaces[0], Reset)
		return
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("  %sEnter number (or comma-separated list, 0 = all):%s ", Bold, Reset)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			fmt.Printf("  %sPlease enter a number or 0 for all.%s\n", Yellow, Reset)
			continue
		}
		if input == "0" || strings.ToLower(input) == "all" {
			a.ScanAll = true
			a.Namespaces = nginxNamespaces
			fmt.Printf("\n  %sSelected: ALL %d ingress-nginx namespace(s)%s\n",
				Green, len(nginxNamespaces), Reset)
			return
		}
		parts := strings.Split(input, ",")
		selected := []string{}
		valid := true
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 1 || idx > len(nginxNamespaces) {
				fmt.Printf("  %sInvalid choice '%s' -- enter numbers between 1 and %d%s\n",
					Red, p, len(nginxNamespaces), Reset)
				valid = false
				break
			}
			selected = append(selected, nginxNamespaces[idx-1])
		}
		if !valid {
			continue
		}
		if len(selected) == 0 {
			fmt.Printf("  %sPlease enter a number or 0 for all.%s\n", Yellow, Reset)
			continue
		}
		a.Namespaces = selected
		if len(selected) == 1 {
			a.Namespace = selected[0]
		} else {
			a.ScanAll = true
		}
		fmt.Printf("\n  %sSelected:%s", Green, Reset)
		for _, ns := range selected {
			fmt.Printf(" %s%s%s", Cyan, ns, Reset)
		}
		fmt.Println()
		return
	}
}

// interactiveSetup clears the screen, shows the banner, collects user config,
// runs the namespace picker, and derives report file names.
func interactiveSetup(a *AuditState) {
	fmt.Print("\033[H\033[2J") // clear screen
	fmt.Printf("%s", Cyan+Bold)
	banner.Printf("NGINX AUDIT")
	fmt.Printf("%s", Reset)
	fmt.Printf("%s  by Rubik MH  |  rubikmh.io%s\n",
		Dim, Reset)
	fmt.Println(strings.Repeat("\u2500", 65))
	fmt.Printf("%sConfiguration Setup%s\n", Bold+Blue, Reset)
	fmt.Println(strings.Repeat("\u2500", 55))
	fmt.Println()
	a.Domain = promptUser("Your domain (e.g. rubikmh.io)", "rubikmh.io")
	a.Email = promptUser("Admin email", fmt.Sprintf("admin@%s", a.Domain))
	fmt.Println()
	a.ControllerName = promptUser("Ingress controller resource name", "ingress-nginx-controller")
	fmt.Printf("  %sThe scanner will look for a Deployment or DaemonSet with this name.%s\n", Dim, Reset)
	fmt.Println()
	pickNamespace(a)
	ts := time.Now().Format("20060102-150405")
	a.TextReportFile = fmt.Sprintf("ingress-audit-%s.txt", ts)
	a.JSONReportFile = fmt.Sprintf("ingress-audit-%s.json", ts)
	time.Sleep(600 * time.Millisecond)
	fmt.Printf("%sConfig saved.%s Running audit for %s%s%s...\n\n",
		Green, Reset, Cyan, a.Domain, Reset)
}

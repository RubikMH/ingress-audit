package main

import (
	"bytes"
	"os/exec"
	"regexp"
	"strings"
)

// kubectl runs a kubectl command and returns trimmed stdout.
func kubectl(args ...string) (string, error) {
	out, err := exec.Command("kubectl", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// kubectlE runs kubectl and returns (stdout, stderr, error).
func kubectlE(args ...string) (string, string, error) {
	cmd := exec.Command("kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// helmCmd runs a helm command and returns trimmed stdout.
func helmCmd(args ...string) (string, error) {
	out, err := exec.Command("helm", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// cmdExists reports whether a program is available on PATH.
func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// stripANSI removes ANSI escape sequences from s (used before writing to files).
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

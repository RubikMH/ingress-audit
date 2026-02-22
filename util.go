package main

import (
	"strings"
)

// nonEmpty filters empty strings from a slice.
func nonEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// extractVersion parses the first semver-like token from a container image
// string (e.g. "registry.k8s.io/ingress-nginx/controller:v1.11.0").
// Returns "unknown" when no version is found.
func extractVersion(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ':' || r == '@' || r == '/'
	})
	// Prefer vX.Y.Z
	for _, p := range parts {
		if len(p) > 1 && p[0] == 'v' {
			sub := strings.SplitN(p, "-", 2)[0]
			if strings.Count(sub, ".") >= 2 {
				return sub
			}
		}
	}
	// Fall back to X.Y.Z (Helm chart versions)
	for _, p := range parts {
		sub := strings.SplitN(p, "-", 2)[0]
		if strings.Count(sub, ".") >= 2 {
			allDigitDot := true
			for _, c := range sub {
				if c != '.' && (c < '0' || c > '9') {
					allDigitDot = false
					break
				}
			}
			if allDigitDot {
				return sub
			}
		}
	}
	return "unknown"
}

// truncate returns the first n characters of s, or s itself if shorter.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// orDefault returns s if non-empty, otherwise "not-set".
func orDefault(s string) string {
	if s == "" {
		return "not-set"
	}
	return s
}

// getMap safely extracts a nested map[string]interface{} from m by key.
// Returns an empty map instead of nil so callers can safely type-assert.
func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	v, _ := m[key].(map[string]interface{})
	if v == nil {
		return map[string]interface{}{}
	}
	return v
}

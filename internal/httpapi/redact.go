package httpapi

import (
	"path/filepath"
	"regexp"
	"strings"
)

// secretPatterns match credential shapes that agents commonly echo: provider
// API keys, bearer tokens, GitHub tokens and key=value assignments. The runner
// already removes the exact values of forwarded credential variables; this is
// the second, shape-based layer for anything else that leaks into a log.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(sk|xai|key|pk|rk)-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API_KEY|TOKEN|SECRET|PASSWORD))(\\?"?\s*[:=]\s*\\?"?)[^\s"\\,}]{8,}`),
}

// redactText replaces credential-shaped substrings with [REDACTED].
func redactText(s string) string {
	for i, re := range secretPatterns {
		switch i {
		case 4:
			s = re.ReplaceAllString(s, "$1 [REDACTED]")
		case 5:
			s = re.ReplaceAllString(s, "$1$2[REDACTED]")
		default:
			s = re.ReplaceAllString(s, "[REDACTED]")
		}
	}
	return s
}

func redactLine(line []byte) []byte { return []byte(redactText(string(line))) }

// dataRel turns an absolute path under the data directory into "$DATA/...".
// Paths outside it collapse to their base name so the server layout is not
// disclosed.
func dataRel(dataDir, path string) string {
	if path == "" {
		return ""
	}
	root, err1 := filepath.Abs(dataDir)
	abs, err2 := filepath.Abs(path)
	if err1 == nil && err2 == nil {
		if rel, err := filepath.Rel(root, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(filepath.Join("$DATA", rel))
		}
	}
	return filepath.Base(path)
}

// redactPaths replaces the data directory prefix inside free text.
func redactPaths(dataDir, text string) string {
	root, err := filepath.Abs(dataDir)
	if err != nil || root == "/" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, root, "$DATA")
}

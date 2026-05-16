// Package terminal provides host terminal detection helpers.
package terminal

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// controlModeCutoffDate is the minimum WezTerm version date that supports
// the tmux control mode protocol (2025-03-08).
var controlModeCutoffDate = [3]int{2025, 3, 8}

// Regexps for the two known WezTerm version formats.
var (
	// Unstable: "0-unstable-YYYY-MM-DD"
	unstableRe = regexp.MustCompile(`^0-unstable-(\d{4})-(\d{2})-(\d{2})$`)
	// Stable: "YYYYMMDD-HHMMSS-hash"
	stableRe = regexp.MustCompile(`^(\d{8})-\d{6}-[0-9a-f]+$`)
)

// SupportsControlMode reports whether the host terminal is WezTerm with a
// version recent enough to support the tmux control mode protocol.
func SupportsControlMode() bool {
	if os.Getenv("TERM_PROGRAM") != "WezTerm" {
		return false
	}
	return versionSupportsControlMode(os.Getenv("TERM_PROGRAM_VERSION"))
}

// versionSupportsControlMode checks a WezTerm version string against the
// minimum cutoff date. Exported for testing via the test file.
func versionSupportsControlMode(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}

	// Try unstable format: 0-unstable-YYYY-MM-DD
	if m := unstableRe.FindStringSubmatch(version); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return !dateBefore(y, mo, d, controlModeCutoffDate)
	}

	// Try stable format: YYYYMMDD-HHMMSS-hash
	if m := stableRe.FindStringSubmatch(version); m != nil {
		dateStr := m[1] // "YYYYMMDD"
		y, _ := strconv.Atoi(dateStr[:4])
		mo, _ := strconv.Atoi(dateStr[4:6])
		d, _ := strconv.Atoi(dateStr[6:8])
		return !dateBefore(y, mo, d, controlModeCutoffDate)
	}

	return false
}

// dateBefore reports whether (y, m, d) is strictly before the cutoff.
func dateBefore(y, m, d int, cutoff [3]int) bool {
	if y != cutoff[0] {
		return y < cutoff[0]
	}
	if m != cutoff[1] {
		return m < cutoff[1]
	}
	return d < cutoff[2]
}

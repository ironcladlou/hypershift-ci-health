// Package releaseconfig owns the shared development defaults. Artifacts record
// resolved values explicitly so historical snapshots do not depend on defaults.
package releaseconfig

import (
	"fmt"
	"strconv"
	"strings"
)

const DevelopmentBranch = "main"
const DevelopmentRelease = "5.1"

func ParseRelease(value string) (int, int, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return 0, 0, false
		}
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func Validate(branch, release string) error {
	if branch == "" {
		return fmt.Errorf("development branch is required")
	}
	if _, _, valid := ParseRelease(release); !valid {
		return fmt.Errorf("development release %q must be a numeric major.minor", release)
	}
	return nil
}

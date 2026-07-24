package codex

import (
	"fmt"
	"regexp"
	"strconv"
)

var versionPattern = regexp.MustCompile(`(?:^|\s)(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?(?:\s|$)`)

type semanticVersion struct {
	major int
	minor int
	patch int
	raw   string
}

func parseVersion(value string) (semanticVersion, error) {
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return semanticVersion{}, fmt.Errorf("could not parse semantic version from %q", value)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return semanticVersion{major: major, minor: minor, patch: patch, raw: match[1] + "." + match[2] + "." + match[3]}, nil
}

func compareVersions(a, b semanticVersion) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch < b.patch {
		return -1
	}
	if a.patch > b.patch {
		return 1
	}
	return 0
}

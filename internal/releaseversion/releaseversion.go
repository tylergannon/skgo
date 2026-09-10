// Package releaseversion maps Conventional Commits to the next semantic version.
package releaseversion

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Level is the semantic version component a set of commits changes.
type Level int

const (
	None Level = iota
	Patch
	Minor
	Major
)

var header = regexp.MustCompile(`^([a-z][a-z0-9-]*)(\([^\r\n()]+\))?(!)?: [^\r\n]+$`)

// ValidateSubject accepts a Conventional Commit subject. Conventional Commits
// intentionally permits project-specific types; Analyze assigns release
// meaning only to the types skgo publishes for.
func ValidateSubject(subject string) error {
	if !header.MatchString(subject) {
		return fmt.Errorf("%q is not a Conventional Commit subject (want type(scope): description)", subject)
	}
	return nil
}

// Analyze returns the largest release change requested by messages. Historical
// non-conventional messages are ignored so adopting the convention does not
// make unreleased commits from before the policy impossible to publish.
func Analyze(messages []string) Level {
	level := None
	for _, message := range messages {
		subject, _, _ := strings.Cut(message, "\n")
		match := header.FindStringSubmatch(subject)
		if match == nil {
			continue
		}
		candidate := None
		if match[3] == "!" || breakingFooter(message) {
			candidate = Major
		} else {
			switch match[1] {
			case "feat":
				candidate = Minor
			case "fix", "perf", "revert":
				candidate = Patch
			}
		}
		if candidate > level {
			level = candidate
		}
	}
	return level
}

// Next increments a stable v-prefixed semantic version.
func Next(base string, level Level) (string, error) {
	canonical := semver.Canonical(base)
	if canonical == "" || semver.Prerelease(canonical) != "" || semver.Build(canonical) != "" {
		return "", fmt.Errorf("%q is not a stable semantic version", base)
	}
	if level == None {
		return "", nil
	}
	parts := strings.Split(strings.TrimPrefix(canonical, "v"), ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	switch level {
	case Major:
		major, minor, patch = major+1, 0, 0
	case Minor:
		minor, patch = minor+1, 0
	case Patch:
		patch++
	default:
		return "", fmt.Errorf("unknown release level %d", level)
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

func breakingFooter(message string) bool {
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(line, "BREAKING CHANGE: ") || strings.HasPrefix(line, "BREAKING-CHANGE: ") {
			return true
		}
	}
	return false
}

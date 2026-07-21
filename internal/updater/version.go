package updater

import (
	"fmt"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major int64
	minor int64
	patch int64
}

// NormalizeVersion accepts release versions in X.Y.Z or vX.Y.Z form and
// returns the canonical vX.Y.Z representation. Development and prerelease
// versions are intentionally rejected: the updater only consumes stable
// GitHub releases.
func NormalizeVersion(version string) (string, error) {
	v, err := parseVersion(version)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch), nil
}

// CompareVersions compares two stable semantic versions.
func CompareVersions(a, b string) (int, error) {
	av, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for _, pair := range [][2]int64{{av.major, bv.major}, {av.minor, bv.minor}, {av.patch, bv.patch}} {
		if pair[0] < pair[1] {
			return -1, nil
		}
		if pair[0] > pair[1] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(version string) (semanticVersion, error) {
	original := strings.TrimSpace(version)
	value := strings.TrimPrefix(original, "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("版本 %q 不是稳定的 SemVer（应为 vX.Y.Z）", original)
	}

	values := make([]int64, 3)
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, fmt.Errorf("版本 %q 不是稳定的 SemVer（应为 vX.Y.Z）", original)
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return semanticVersion{}, fmt.Errorf("版本 %q 不是稳定的 SemVer（应为 vX.Y.Z）", original)
			}
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return semanticVersion{}, fmt.Errorf("解析版本 %q: %w", original, err)
		}
		values[i] = n
	}

	return semanticVersion{major: values[0], minor: values[1], patch: values[2]}, nil
}

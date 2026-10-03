package manifest

import (
	"cmp"
	"strings"
)

// ValidVersion reports whether v is a semver 2.0.0 version: MAJOR.MINOR.PATCH, then an optional
// "-prerelease" and "+build", with no leading zeros in numbers and no "v".
func ValidVersion(v string) bool {
	if v == "" || len(v) > maxVersion {
		return false
	}
	v, build, hasBuild := strings.Cut(v, "+")
	core, pre, hasPre := strings.Cut(v, "-")
	nums := strings.Split(core, ".")
	if len(nums) != 3 || hasPre && !identifiers(pre, true) || hasBuild && !identifiers(build, false) {
		return false
	}
	for _, n := range nums {
		if !number(n) {
			return false
		}
	}
	return true
}

// identifiers checks dot-separated semver identifiers; prerelease numbers can't lead with zero.
func identifiers(s string, prerelease bool) bool {
	for id := range strings.SplitSeq(s, ".") {
		switch {
		case id == "" || strings.ContainsFunc(id, func(c rune) bool { return !lower(c) && !digit(c) && (c < 'A' || c > 'Z') && c != '-' }):
			return false
		case prerelease && decimal(id) && !number(id):
			return false
		}
	}
	return true
}

// number is a decimal with no leading zero.
func number(s string) bool { return decimal(s) && (s == "0" || s[0] != '0') }

// CompareVersions orders two valid versions by semver precedence: -1, 0 or +1. Build metadata
// doesn't count.
func CompareVersions(a, b string) int {
	a, _, _ = strings.Cut(a, "+")
	b, _, _ = strings.Cut(b, "+")
	aCore, aPre, aHasPre := strings.Cut(a, "-")
	bCore, bPre, bHasPre := strings.Cut(b, "-")
	if c := compareIDs(strings.Split(aCore, "."), strings.Split(bCore, ".")); c != 0 {
		return c
	}
	switch {
	case aHasPre && !bHasPre:
		return -1
	case !aHasPre && bHasPre:
		return 1
	}
	return compareIDs(strings.Split(aPre, "."), strings.Split(bPre, "."))
}

// compareIDs orders dot-separated identifiers: numbers by value and below text, text by bytes,
// and a shorter list first when the rest are equal.
func compareIDs(a, b []string) int {
	for i := range min(len(a), len(b)) {
		if c := compareID(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

func compareID(a, b string) int {
	aNum, bNum := decimal(a), decimal(b)
	switch {
	case aNum && bNum:
		return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b))
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}

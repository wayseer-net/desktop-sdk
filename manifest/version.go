package manifest

import "strings"

// validVersion checks a semver 2.0.0 version: MAJOR.MINOR.PATCH, then an optional
// "-prerelease" and "+build", with no leading zeros in numbers and no "v".
func validVersion(v string) bool {
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

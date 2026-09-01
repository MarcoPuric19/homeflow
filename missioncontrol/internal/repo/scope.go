// Package repo connects tickets to the repository they are about.
//
// Two halves, because the repository has two halves of history:
//
//   - Files: a ticket names the paths it works on. That works today and gives
//     the collision warning Austausch.md's claim table gives by hand.
//   - Commits: a commit whose message mentions a ticket key belongs to that
//     ticket. That works from the first commit onwards - this repository has
//     none yet, and the cockpit says so instead of pretending.
package repo

import (
	"path"
	"strings"
)

// Matches reports whether a file path is covered by a scope pattern.
//
// Three forms, in the order people actually write them:
//
//	backend/internal/pets/      a directory - everything below it
//	backend/**/*_test.go        ** spans directories, * stays inside one
//	Makefile                    an exact path
func Matches(pattern, file string) bool {
	pattern = strings.TrimSpace(strings.TrimPrefix(pattern, "./"))
	file = strings.TrimSpace(strings.TrimPrefix(file, "./"))
	if pattern == "" || file == "" {
		return false
	}
	if pattern == file {
		return true
	}
	// A bare directory covers everything underneath, with or without a slash.
	if !strings.ContainsAny(pattern, "*?[") {
		prefix := strings.TrimSuffix(pattern, "/") + "/"
		return strings.HasPrefix(file, prefix)
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

// matchSegments walks both paths segment by segment; "**" may swallow any
// number of segments, which is what makes `backend/**/*_test.go` work.
func matchSegments(pattern, file []string) bool {
	if len(pattern) == 0 {
		return len(file) == 0
	}
	if pattern[0] == "**" {
		// Either ** matches nothing, or it swallows one segment and retries.
		if matchSegments(pattern[1:], file) {
			return true
		}
		if len(file) == 0 {
			return false
		}
		return matchSegments(pattern, file[1:])
	}
	if len(file) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], file[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], file[1:])
}

// Overlap reports whether two scopes can touch the same file. It is
// deliberately generous: a warning that fires once too often costs a glance, a
// missing one costs an overwrite.
func Overlap(left, right []string) []string {
	var hits []string
	for _, a := range left {
		for _, b := range right {
			if scopesTouch(a, b) {
				hits = append(hits, a+" ↔ "+b)
			}
		}
	}
	return hits
}

func scopesTouch(a, b string) bool {
	if a == b {
		return true
	}
	if Matches(a, b) || Matches(b, a) {
		return true
	}
	// Two patterns can overlap without either matching the other literally
	// (backend/** and **/pets). Compare their fixed prefixes.
	return strings.HasPrefix(fixedPrefix(a), fixedPrefix(b)) || strings.HasPrefix(fixedPrefix(b), fixedPrefix(a))
}

// fixedPrefix is the part of a pattern before the first wildcard.
func fixedPrefix(pattern string) string {
	segments := strings.Split(pattern, "/")
	var out []string
	for _, segment := range segments {
		if strings.ContainsAny(segment, "*?[") {
			break
		}
		out = append(out, segment)
	}
	return strings.Join(out, "/")
}

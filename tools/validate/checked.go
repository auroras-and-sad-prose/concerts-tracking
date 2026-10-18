// checked.json records, per tracked artist, when the concert-watch routine
// began the last run that read all of that artist's sources to the end. The
// page shows it as "Last checked", so a quiet run, which finds nothing to put
// in seen.json, still moves the date forward: the concerts are current as of
// that time.
//
// It is the one file that may reach main without a person merging it: a stamp
// PR that changes nothing else is merged by the stamp-merge workflow (see
// "When the data was last checked" in CLAUDE.md). That is why it is kept apart
// from seen.json and travel.json, whose changes all go through a reviewed PR.
// Because no person reviews a stamp, the checks here, which the workflow runs
// before merging, are what stand between a bad one and the page: a timestamp
// has one exact shape, it cannot lie in the future, and it can only be for an
// artist the roster knows.
package main

import (
	"fmt"
	"slices"
	"time"
)

// checkedLayout is the only accepted timestamp shape: UTC, to the second, with
// a literal Z. One shape means one comparison rule and nothing for the page to
// guess at.
const checkedLayout = "2006-01-02T15:04:05Z"

// checkedClockSkew is how far ahead of the validating machine's clock a stamp
// may be before it is called a lie. The routine and CI run on different
// machines.
const checkedClockSkew = 10 * time.Minute

// Checked is the top-level shape of checked.json.
type Checked struct {
	// Artists maps an artist slug to the UTC time that artist's sources were
	// last read in full.
	Artists map[string]string `json:"artists"`
}

// ValidateChecked checks checked.json in isolation and, when roster is
// non-nil, against artists.json. An empty file is fine: it is the state before
// the first run has stamped anything, and a tracked artist with no entry is
// shown as not checked yet rather than failing, so adding someone to the roster
// never turns CI red. An entry for an artist who is no longer tracked is fine
// for the same reason rows of theirs stay in seen.json; the page ignores it.
func ValidateChecked(c Checked, roster *Artists, now time.Time) []string {
	var problems []string

	slugs := make([]string, 0, len(c.Artists))
	for slug := range c.Artists {
		slugs = append(slugs, slug)
	}
	slices.Sort(slugs)

	known := map[string]bool{}
	if roster != nil {
		for _, a := range roster.Artists {
			known[a.Slug] = true
		}
	}

	for _, slug := range slugs {
		if roster != nil && !known[slug] {
			problems = append(problems, fmt.Sprintf("artists[%q] is not in the roster", slug))
		}
		at, err := time.Parse(checkedLayout, c.Artists[slug])
		switch {
		case err != nil || at.Format(checkedLayout) != c.Artists[slug]:
			problems = append(problems, fmt.Sprintf("artists[%q] %q is not a UTC timestamp like 2026-10-16T05:12:00Z", slug, c.Artists[slug]))
		case at.After(now.Add(checkedClockSkew)):
			problems = append(problems, fmt.Sprintf("artists[%q] %q is in the future", slug, c.Artists[slug]))
		}
	}
	return problems
}

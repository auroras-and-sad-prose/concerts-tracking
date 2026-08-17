// artists.json is the curated roster: which musicians are tracked, and which
// instrument(s) each one plays. It is deliberately *not* derived from the
// concert pages — an instrument is a stable fact about a performer, not
// something that changes per event — so the concert-watch routine never writes
// this file. Adding an artist, or correcting an instrument, is a reviewed
// change to the repo, exactly like extending the location or domain allowlists.
//
// The checks here keep the roster and seen.json from drifting apart: every
// concert's artist must be registered, under the same name and slug the row
// itself uses, so a typo'd or newly-introduced artist fails CI rather than
// quietly rendering with no instrument.
package main

import (
	"fmt"
	"strings"
)

// allowedInstruments is the closed vocabulary for the instruments field, like
// allowedTags is for location_tag. Tracking a musician who plays something not
// listed here means extending this set in a reviewed change.
var allowedInstruments = map[string]bool{
	"piano":  true,
	"violin": true,
}

// Artist is one entry in artists.json. slug is the identity key, and matches
// the first segment of a concert id ("scheps|2026-09-12|altenkrempe").
type Artist struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Instruments []string `json:"instruments"`
}

// Artists is the top-level shape of artists.json.
type Artists struct {
	Artists []Artist `json:"artists"`
}

// ValidateArtists checks the roster file in isolation: well-formed slugs and
// names, a non-empty instrument list drawn from the allowed vocabulary, and no
// duplicate slugs or names (both are used as lookup keys, by the cross-check
// below and by the page's instrument filter respectively).
func ValidateArtists(a Artists) []string {
	var problems []string

	if len(a.Artists) == 0 {
		return []string{"artists: roster is empty"}
	}

	slugs := make(map[string]int, len(a.Artists))
	names := make(map[string]int, len(a.Artists))
	for i, ar := range a.Artists {
		label := ar.Slug
		if label == "" {
			label = fmt.Sprintf("artists[%d]", i)
		}

		if !slugRe.MatchString(ar.Slug) {
			problems = append(problems, fmt.Sprintf("%s: slug %q must be lowercase letters", label, ar.Slug))
		} else if prev, dup := slugs[ar.Slug]; dup {
			problems = append(problems, fmt.Sprintf("%s: duplicate slug (also artists[%d])", label, prev))
		} else {
			slugs[ar.Slug] = i
		}

		if strings.TrimSpace(ar.Name) == "" {
			problems = append(problems, fmt.Sprintf("%s: field %q is required but empty", label, "name"))
		} else if prev, dup := names[ar.Name]; dup {
			problems = append(problems, fmt.Sprintf("%s: duplicate name %q (also artists[%d])", label, ar.Name, prev))
		} else {
			names[ar.Name] = i
		}

		if len(ar.Instruments) == 0 {
			problems = append(problems, fmt.Sprintf("%s: field %q is required (every artist plays at least one instrument)", label, "instruments"))
		}
		seen := make(map[string]bool, len(ar.Instruments))
		for j, ins := range ar.Instruments {
			switch {
			case !allowedInstruments[ins]:
				problems = append(problems, fmt.Sprintf("%s: instruments[%d] %q is not in the allowed set", label, j, ins))
			case seen[ins]:
				problems = append(problems, fmt.Sprintf("%s: instruments lists %q twice", label, ins))
			default:
				seen[ins] = true
			}
		}
	}

	return problems
}

// CheckRoster ties seen.json to the roster: a concert's id slug must name a
// registered artist, and that artist's name must be exactly the row's artist
// field. Concerts whose id is malformed are skipped here — Validate already
// reports those, and re-reporting them as "unknown artist" would be noise.
//
// The reverse direction is intentionally not checked: a newly tracked artist
// legitimately sits in the roster with no concerts recorded yet.
func CheckRoster(f File, a Artists) []string {
	var problems []string

	bySlug := make(map[string]Artist, len(a.Artists))
	for _, ar := range a.Artists {
		bySlug[ar.Slug] = ar
	}

	for i, c := range f.Concerts {
		label := c.ID
		if label == "" {
			label = fmt.Sprintf("concerts[%d]", i)
		}
		parts := strings.Split(c.ID, "|")
		if len(parts) != 3 {
			continue
		}
		ar, ok := bySlug[parts[0]]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s: artist slug %q is not in artists.json; add the artist (with instruments) there first", label, parts[0]))
			continue
		}
		if ar.Name != c.Artist {
			problems = append(problems, fmt.Sprintf(
				"%s: artist %q does not match artists.json name %q for slug %q", label, c.Artist, ar.Name, ar.Slug))
		}
	}

	return problems
}

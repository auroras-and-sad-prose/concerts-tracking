// Command validate checks seen.json for structural integrity so that a
// hallucinated or malformed entry produced by the concert-watch routine fails
// CI instead of silently landing in the dataset.
//
// It enforces:
//   - the file matches the expected schema exactly (no missing/unknown fields);
//   - every required field is present and non-empty;
//   - date and first_seen are real, zero-padded YYYY-MM-DD calendar dates;
//   - concert dates fall within a sane window around "now" (catches typoed years);
//   - location_tag is drawn from a fixed vocabulary;
//   - source_url is http(s) and points at an allowlisted domain;
//   - pieces is either a non-empty array of work titles or a descriptive string;
//   - id has the canonical "<slug>|<date>|<city>" shape consistent with its row;
//   - ids are unique.
//
// With -base pointing at the previous version of the file, it additionally
// enforces two rules on entries that already existed:
//
//   - entries may not be deleted, and the fields that pin a row to one specific
//     concert (artist, date, city) plus its provenance (first_seen) may not
//     change. Rewriting one of those would silently turn a vetted row into a
//     different concert while keeping its id;
//   - descriptive fields (venue, program, pieces, ...) may be refined as more is
//     learned about an event, but a field that already carried information may
//     not be emptied back out. Detail can be added or corrected, never erased.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// allowedTags is the closed vocabulary for location_tag. Extend deliberately
// (and via review) when a genuinely new region is tracked.
var allowedTags = map[string]bool{
	"europe":  true,
	"germany": true,
	"berlin":  true,
}

// allowedHosts is the set of registrable domains a source_url may point at,
// compared after stripping a leading "www.". Anything else is treated as an
// untrusted (possibly fabricated) source.
var allowedHosts = map[string]bool{
	"ilyunburkev.com":       true,
	"olgascheps.com":        true,
	"mariaduenasviolin.com": true,
	"mayaoganyan.com":       true,
	"janinejansen.com":      true,
	"juliafischer.com":      true,
	"itzhakperlman.com":     true,
	"bachtrack.com":         true,
}

const dateLayout = "2006-01-02"

var (
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	slugRe = regexp.MustCompile(`^[a-z]+$`)
)

// Pieces records the individual works to be performed. It is deliberately a
// union of two JSON shapes:
//
//	["Chopin Ballade No. 1", "Chopin Ballade No. 2"]   works known individually
//	"Composers only: Brahms, Schubert"                 works not listed by source
//
// The string form exists so the routine is never cornered into inventing work
// titles just to produce an array: when a source names a conductor and two
// composers but no repertoire, the honest answer is a sentence, not a list.
type Pieces struct {
	List []string // non-nil when the JSON value was an array
	Text *string  // non-nil when the JSON value was a string
}

// UnmarshalJSON accepts an array of strings, a string, or null, and rejects
// every other shape rather than coercing it.
func (p *Pieces) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) == 0 || string(trimmed) == "null":
		*p = Pieces{}
		return nil
	case trimmed[0] == '[':
		// Decoded via *string so a null element is caught here rather than
		// silently becoming an empty title.
		var raw []*string
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return fmt.Errorf("pieces array must contain only strings: %w", err)
		}
		list := make([]string, 0, len(raw)) // non-nil: "empty array" is not "absent"
		for i, item := range raw {
			if item == nil {
				return fmt.Errorf("pieces[%d] is null; pieces array must contain only strings", i)
			}
			list = append(list, *item)
		}
		*p = Pieces{List: list}
		return nil
	case trimmed[0] == '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*p = Pieces{Text: &s}
		return nil
	default:
		return fmt.Errorf("pieces must be an array of work titles or a string, got %s", trimmed)
	}
}

func (p Pieces) MarshalJSON() ([]byte, error) {
	switch {
	case p.List != nil:
		return json.Marshal(p.List)
	case p.Text != nil:
		return json.Marshal(*p.Text)
	default:
		return []byte("null"), nil
	}
}

// Present reports whether the field carries any value at all.
func (p Pieces) Present() bool { return p.List != nil || p.Text != nil }

// Concert mirrors one entry in seen.json. Nullable fields use *string so a JSON
// null is accepted; required fields use string and are checked for emptiness.
type Concert struct {
	ID          string  `json:"id"`
	Artist      string  `json:"artist"`
	Date        string  `json:"date"`
	City        string  `json:"city"`
	Country     string  `json:"country"`
	Venue       *string `json:"venue"`
	Program     *string `json:"program"`
	Pieces      Pieces  `json:"pieces"`
	LocationTag string  `json:"location_tag"`
	SourceURL   string  `json:"source_url"`
	FirstSeen   string  `json:"first_seen"`
}

// File is the top-level shape of seen.json.
type File struct {
	Concerts []Concert `json:"concerts"`
}

func main() {
	filePath := flag.String("file", "seen.json", "path to the concerts JSON file to validate")
	basePath := flag.String("base", "", "optional path to the previous version of the file; enables the append-only check")
	flag.Parse()

	f, err := loadFile(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read %s: %v\n", *filePath, err)
		os.Exit(1)
	}

	var base *File
	if *basePath != "" {
		b, err := loadFile(*basePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot read base %s: %v\n", *basePath, err)
			os.Exit(1)
		}
		base = &b
	}

	problems := Validate(f, base, time.Now().UTC())
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "%s: %d problem(s) found:\n", *filePath, len(problems))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		os.Exit(1)
	}
	fmt.Printf("%s: OK (%d concerts)\n", *filePath, len(f.Concerts))
}

// loadFile decodes a concerts file, rejecting any unknown fields so that a
// stray or hallucinated key surfaces as an error rather than being ignored.
func loadFile(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return File{}, err
	}
	return f, nil
}

// Validate returns a human-readable list of every problem found. An empty slice
// means the file is valid. now is injected so the year-window check is testable.
func Validate(f File, base *File, now time.Time) []string {
	var problems []string

	seen := make(map[string]int, len(f.Concerts))
	for i, c := range f.Concerts {
		label := c.ID
		if label == "" {
			label = fmt.Sprintf("concerts[%d]", i)
		}

		for _, req := range []struct {
			name, val string
		}{
			{"id", c.ID}, {"artist", c.Artist}, {"date", c.Date}, {"city", c.City},
			{"country", c.Country}, {"location_tag", c.LocationTag},
			{"source_url", c.SourceURL}, {"first_seen", c.FirstSeen},
		} {
			if strings.TrimSpace(req.val) == "" {
				problems = append(problems, fmt.Sprintf("%s: field %q is required but empty", label, req.name))
			}
		}

		if !validDate(c.Date) {
			problems = append(problems, fmt.Sprintf("%s: date %q is not a valid YYYY-MM-DD date", label, c.Date))
		} else if y := yearOf(c.Date); y < now.Year()-1 || y > now.Year()+3 {
			problems = append(problems, fmt.Sprintf("%s: date year %d is outside the expected window %d..%d (possible typo)", label, y, now.Year()-1, now.Year()+3))
		}
		if !validDate(c.FirstSeen) {
			problems = append(problems, fmt.Sprintf("%s: first_seen %q is not a valid YYYY-MM-DD date", label, c.FirstSeen))
		}

		if c.LocationTag != "" && !allowedTags[c.LocationTag] {
			problems = append(problems, fmt.Sprintf("%s: location_tag %q is not in the allowed set", label, c.LocationTag))
		}

		if c.SourceURL != "" && !allowedHost(c.SourceURL) {
			problems = append(problems, fmt.Sprintf("%s: source_url %q is not http(s) on an allowlisted domain", label, c.SourceURL))
		}

		if msg := checkPieces(c.Pieces); msg != "" {
			problems = append(problems, fmt.Sprintf("%s: %s", label, msg))
		}

		if c.ID != "" {
			if msg := checkID(c.ID, c.Date, c.City); msg != "" {
				problems = append(problems, fmt.Sprintf("%s: %s", label, msg))
			}
			if prev, dup := seen[c.ID]; dup {
				problems = append(problems, fmt.Sprintf("%s: duplicate id (also concerts[%d])", label, prev))
			} else {
				seen[c.ID] = i
			}
		}
	}

	if base != nil {
		problems = append(problems, checkAppendOnly(*base, f)...)
	}

	return problems
}

func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return false
	}
	return t.Format(dateLayout) == s // rejects normalized invalids like 2026-02-30
}

func yearOf(s string) int {
	t, _ := time.Parse(dateLayout, s)
	return t.Year()
}

func allowedHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return allowedHosts[host]
}

// checkPieces enforces that the field is populated and well-formed. It is
// required on every row: an unknown programme is stated in words, never left
// silent, so that a row the routine simply failed to fill in is distinguishable
// from one whose source genuinely lists no repertoire.
func checkPieces(p Pieces) string {
	switch {
	case p.List != nil:
		if len(p.List) == 0 {
			return `pieces is an empty array; use a string such as "Programme not announced" instead`
		}
		for i, item := range p.List {
			if strings.TrimSpace(item) == "" {
				return fmt.Sprintf("pieces[%d] is empty", i)
			}
		}
	case p.Text != nil:
		if strings.TrimSpace(*p.Text) == "" {
			return `pieces is an empty string; use a string such as "Programme not announced"`
		}
	default:
		return `field "pieces" is required (an array of work titles, or a string such as "Programme not announced")`
	}
	return ""
}

// checkID verifies the id has the canonical "<slug>|<date>|<city>" shape and
// that its date and city segments agree with the row's own fields.
func checkID(id, date, city string) string {
	parts := strings.Split(id, "|")
	if len(parts) != 3 {
		return fmt.Sprintf("id %q must have the form <slug>|<date>|<city>", id)
	}
	if !slugRe.MatchString(parts[0]) {
		return fmt.Sprintf("id slug %q must be lowercase letters", parts[0])
	}
	if parts[1] != date {
		return fmt.Sprintf("id date segment %q does not match date field %q", parts[1], date)
	}
	if want := strings.ToLower(city); parts[2] != want {
		return fmt.Sprintf("id city segment %q does not match city field %q (lowercased)", parts[2], want)
	}
	return ""
}

// immutableFields pin a row to one specific concert, plus the date we first
// recorded it. Changing any of them would quietly repoint a vetted row at a
// different event while keeping its id, so they are frozen once written. (id
// itself is the identity key, and the checkID rule ties it to date and city.)
var immutableFields = []struct {
	name string
	get  func(Concert) string
}{
	{"artist", func(c Concert) string { return c.Artist }},
	{"date", func(c Concert) string { return c.Date }},
	{"city", func(c Concert) string { return c.City }},
	{"first_seen", func(c Concert) string { return c.FirstSeen }},
}

// refinableFields are descriptive: a venue gets announced, a programme listed
// only by composer later names its works. They may be rewritten freely, subject
// only to the rule below that information is never erased once recorded.
var refinableFields = []struct {
	name     string
	populate func(Concert) bool
}{
	{"venue", func(c Concert) bool { return c.Venue != nil }},
	{"program", func(c Concert) bool { return c.Program != nil }},
	{"pieces", func(c Concert) bool { return c.Pieces.Present() }},
}

// checkAppendOnly ensures every entry present in base still exists in head with
// its identity intact. New entries are allowed, and descriptive detail may be
// refined as more is learned; deletions, identity rewrites, and blanking out a
// field that already had a value are not.
func checkAppendOnly(base, head File) []string {
	var problems []string
	headByID := make(map[string]Concert, len(head.Concerts))
	for _, c := range head.Concerts {
		headByID[c.ID] = c
	}
	for _, b := range base.Concerts {
		h, ok := headByID[b.ID]
		if !ok {
			problems = append(problems, fmt.Sprintf("append-only: existing entry %q was deleted", b.ID))
			continue
		}
		for _, f := range immutableFields {
			if was, now := f.get(b), f.get(h); was != now {
				problems = append(problems, fmt.Sprintf(
					"append-only: existing entry %q changed immutable field %s (%q -> %q)", b.ID, f.name, was, now))
			}
		}
		for _, f := range refinableFields {
			if f.populate(b) && !f.populate(h) {
				problems = append(problems, fmt.Sprintf(
					"append-only: existing entry %q cleared %s; detail may be refined but not erased", b.ID, f.name))
			}
		}
	}
	return problems
}

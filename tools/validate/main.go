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
//   - id has the canonical "<slug>|<date>|<city>" shape consistent with its row;
//   - ids are unique.
//
// With -base pointing at the previous version of the file, it additionally
// enforces that the change is append-only: existing entries may not be modified
// or deleted, only new ones added. This keeps a bad generation from corrupting
// rows that were already vetted.
package main

import (
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
	"bachtrack.com":         true,
}

const dateLayout = "2006-01-02"

var (
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	slugRe = regexp.MustCompile(`^[a-z]+$`)
)

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

// checkAppendOnly ensures every entry present in base still exists unchanged in
// head. New entries are allowed; modifications and deletions are not.
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
		if !sameConcert(b, h) {
			problems = append(problems, fmt.Sprintf("append-only: existing entry %q was modified", b.ID))
		}
	}
	return problems
}

func sameConcert(a, b Concert) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

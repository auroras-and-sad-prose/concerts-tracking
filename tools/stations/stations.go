// Package stations looks up German railway stations by name in de.csv, a
// subset of the open trainline-eu/stations dataset holding each German
// station's name and its Deutsche Bahn station number (its EVA number).
//
// The page needs that number to build a bahn.de search that opens on the
// right destination: bahn.de ignores a destination given by name alone. The
// concert-watch routine can't reach DB's own station search, which blocks
// automated requests, so it finds the number here instead, and CI checks every
// station travel.json names against this same file. A number a run recalled
// rather than looked up would point bahn.de at some other stop, and no page
// would look wrong until someone tried to travel.
//
// See README.md in this directory for the file's source, licence and how to
// regenerate it.
package stations

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Station is one row of de.csv.
type Station struct {
	Name string `json:"name"`
	EVA  string `json:"eva"`
}

// List is the loaded file, indexed for exact lookups.
type List struct {
	all    []Station
	byName map[string][]Station
}

var evaRe = regexp.MustCompile(`^[0-9]{6,7}$`)

// ValidEVA reports whether s has the shape of a DB station number: six or
// seven digits, which is every number the dataset holds.
func ValidEVA(s string) bool { return evaRe.MatchString(s) }

// Load reads a "name;eva" file with a header line.
func Load(path string) (*List, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	l := &List{byName: map[string][]Station{}}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if line == 1 {
			if text != "name;eva" {
				return nil, fmt.Errorf("%s: header is %q, want %q", path, text, "name;eva")
			}
			continue
		}
		name, eva, ok := strings.Cut(text, ";")
		if !ok || strings.TrimSpace(name) == "" || !ValidEVA(eva) {
			return nil, fmt.Errorf("%s:%d: malformed row %q", path, line, text)
		}
		s := Station{Name: name, EVA: eva}
		l.all = append(l.all, s)
		l.byName[name] = append(l.byName[name], s)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(l.all) == 0 {
		return nil, fmt.Errorf("%s: no stations", path)
	}
	return l, nil
}

// Has reports whether the list holds exactly this name with this number.
func (l *List) Has(s Station) bool {
	for _, c := range l.byName[s.Name] {
		if c.EVA == s.EVA {
			return true
		}
	}
	return false
}

// Result is the outcome of Find: the one station to use, or every candidate
// that tied, or neither when nothing matched.
type Result struct {
	Station    *Station
	Candidates []Station
	Rule       string
}

// Find picks the station for a place name, trying each rule in turn and
// stopping at the first that matches anything:
//
//  1. "<name> Hbf" — the main station, where a city has one;
//  2. "<name>" exactly;
//  3. "<name> Bf";
//  4. "<name> (<qualifier>)", optionally followed by " Hbf" — the form DB
//     uses to tell same-named places apart: "Kempen (Niederrhein)".
//
// A rule that matches more than one station settles nothing, so Find returns
// the tie rather than choosing: "Neustadt" matches a dozen qualified stations,
// and picking one would send the reader to the wrong town. The caller retries
// with a fuller name ("Frankfurt (Main)") or leaves the city without one.
func (l *List) Find(name string) Result {
	name = strings.TrimSpace(name)
	if name == "" {
		return Result{}
	}
	for _, rule := range []struct {
		label string
		names []string
	}{
		{"main station", []string{name + " Hbf"}},
		{"exact name", []string{name}},
		{"station", []string{name + " Bf"}},
	} {
		var hits []Station
		for _, n := range rule.names {
			hits = append(hits, l.byName[n]...)
		}
		if r, ok := settle(hits, rule.label); ok {
			return r
		}
	}

	prefix := name + " ("
	var hits []Station
	for _, s := range l.all {
		rest, ok := strings.CutPrefix(s.Name, prefix)
		if !ok {
			continue
		}
		// The qualifier closes the name, or is followed only by " Hbf".
		if i := strings.Index(rest, ")"); i > 0 && (rest[i+1:] == "" || rest[i+1:] == " Hbf") {
			hits = append(hits, s)
		}
	}
	r, _ := settle(hits, "qualified name")
	return r
}

func settle(hits []Station, rule string) (Result, bool) {
	switch len(hits) {
	case 0:
		return Result{}, false
	case 1:
		return Result{Station: &hits[0], Rule: rule}, true
	default:
		return Result{Candidates: hits, Rule: rule}, true
	}
}

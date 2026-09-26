// travel.json holds, for each German city a concert is in, what the page needs
// to help the reader get there from Berlin: roughly how long the quickest
// train takes, and the city's railway station, from which the page builds a
// bahn.de search for Deutschlandticket connections. Nothing in seen.json
// refers to it.
//
// Unlike artists.json and favorites.json this file IS written by the
// concert-watch routine (see "Train times from Berlin" in CLAUDE.md), and each
// half of an entry is copied from one named source, never estimated:
//
//   - train: the quickest train route the Rome2Rio connector returned from
//     Berlin Hbf — its name, duration and operators — with the query that was
//     sent and the day, so a person can repeat it. The page reads the
//     operators to say whether the route is regional only, and so covered by a
//     Deutschlandticket; that verdict is derived there, never stored here.
//   - station: the city's station name and DB station number, found with
//     -find-station in tools/stations/de.csv. CI checks the pair against that
//     same file, so a number recalled rather than looked up fails the build.
//
// Either half may be missing — the connector may be unavailable to a run, or
// a village may have no station — but not both. What else can be checked is
// that the route is a train route rather than the drive or the flight Rome2Rio
// lists beside it, and that the entry is for a city some concert in the
// dataset is in, so a misspelt city, which the page could never match to a
// card, fails the build instead of sitting unused.
package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/stations"
)

// maxTravelMinutes bounds a plausible duration. Nothing in Germany is a day's
// train ride from Berlin, so a larger figure is a unit mix-up (seconds, or
// hours and minutes run together), not a slow connection.
const maxTravelMinutes = 24 * 60

// TravelTime is one entry in travel.json.
type TravelTime struct {
	// City is the concert's city exactly as seen.json spells it; the page
	// joins on it.
	City    string            `json:"city"`
	Station *stations.Station `json:"station"`
	Train   *TrainRoute       `json:"train"`
}

// TrainRoute is what Rome2Rio returned for the quickest train route.
type TrainRoute struct {
	// Query is the destination sent to Rome2Rio, which may be fuller than
	// City ("Frankfurt am Main, Germany" for "Frankfurt").
	Query string `json:"query"`
	// Minutes is the duration Rome2Rio gave for Route.
	Minutes int `json:"minutes"`
	// Route is Rome2Rio's own name for the route, copied as returned:
	// "Train", "Train via Wolfsburg", "Train, bus".
	Route string `json:"route"`
	// Carriers are the operators Rome2Rio listed for Route, copied as
	// returned.
	Carriers []string `json:"carriers"`
	// Checked is the day the query was run.
	Checked string `json:"checked"`
}

// Travel is the top-level shape of travel.json.
type Travel struct {
	Cities []TravelTime `json:"cities"`
}

// ValidateTravel checks travel.json in isolation and against seen.json and,
// when list is non-nil, against the station list. An empty file is fine: it
// is the state before any run has filled it in.
func ValidateTravel(t Travel, f File, list *stations.List) []string {
	var problems []string

	// Any row's city, whatever its location_tag. A row's city is frozen and
	// rows are never deleted, so an entry that once passed can't start
	// failing; location_tag, by contrast, is refinable, and tying the check
	// to it would turn a later retag (Potsdam moved to berlin) into a red
	// build on an entry the routine may never remove. An entry for a city
	// with no germany row left is simply not shown.
	knownCities := map[string]bool{}
	for _, c := range f.Concerts {
		knownCities[c.City] = true
	}

	cities := make(map[string]int, len(t.Cities))
	for i, e := range t.Cities {
		label := e.City
		if strings.TrimSpace(label) == "" {
			label = fmt.Sprintf("cities[%d]", i)
			problems = append(problems, fmt.Sprintf("%s: field %q is required but empty", label, "city"))
		} else {
			if prev, dup := cities[e.City]; dup {
				problems = append(problems, fmt.Sprintf("%s: duplicate city (also cities[%d])", label, prev))
			} else {
				cities[e.City] = i
			}
			if !knownCities[e.City] {
				problems = append(problems, fmt.Sprintf(
					"%s: no concert in seen.json has this city; the name must match a row's city exactly", label))
			}
		}

		if e.Station == nil && e.Train == nil {
			problems = append(problems, fmt.Sprintf(
				"%s: has neither a station nor a train route; leave the city out until a run finds one", label))
		}
		if e.Station != nil {
			for _, m := range checkStation(*e.Station, list) {
				problems = append(problems, fmt.Sprintf("%s: station: %s", label, m))
			}
		}
		if e.Train != nil {
			for _, m := range checkTrain(*e.Train) {
				problems = append(problems, fmt.Sprintf("%s: train: %s", label, m))
			}
		}
	}

	return problems
}

func checkStation(s stations.Station, list *stations.List) []string {
	var problems []string
	if strings.TrimSpace(s.Name) == "" {
		problems = append(problems, `field "name" is required but empty`)
	}
	if !stations.ValidEVA(s.EVA) {
		problems = append(problems, fmt.Sprintf("eva %q is not a 6- or 7-digit DB station number", s.EVA))
	}
	if len(problems) == 0 && list != nil && !list.Has(s) {
		problems = append(problems, fmt.Sprintf(
			"%q with number %s is not in tools/stations/de.csv; copy both from -find-station", s.Name, s.EVA))
	}
	return problems
}

func checkTrain(r TrainRoute) []string {
	var problems []string
	for _, req := range []struct{ name, val string }{
		{"query", r.Query}, {"route", r.Route}, {"checked", r.Checked},
	} {
		if strings.TrimSpace(req.val) == "" {
			problems = append(problems, fmt.Sprintf("field %q is required but empty", req.name))
		}
	}

	if r.Minutes < 1 || r.Minutes > maxTravelMinutes {
		problems = append(problems, fmt.Sprintf("minutes %d is outside 1..%d", r.Minutes, maxTravelMinutes))
	}

	// Rome2Rio names every option by its first mode — "Drive", "Fly to …",
	// "Bus", "Night train", "Train via …" — so a route that doesn't open with
	// "Train" is one of the others, copied from the wrong line.
	if r.Route != "" && !strings.HasPrefix(r.Route, "Train") {
		problems = append(problems, fmt.Sprintf(
			"route %q is not a train route (Rome2Rio names those \"Train …\")", r.Route))
	}

	// Every train route Rome2Rio returns names who runs it, so an empty list
	// means the field was dropped, not that nobody does.
	if len(r.Carriers) == 0 {
		problems = append(problems, `field "carriers" is required (the operators Rome2Rio listed for the route)`)
	}
	for j, name := range r.Carriers {
		switch {
		case strings.TrimSpace(name) == "":
			problems = append(problems, fmt.Sprintf("carriers[%d] is empty", j))
		case slices.Contains(r.Carriers[:j], name):
			problems = append(problems, fmt.Sprintf("carriers lists %q twice", name))
		}
	}

	if r.Checked != "" && !validDate(r.Checked) {
		problems = append(problems, fmt.Sprintf("checked %q is not a valid YYYY-MM-DD date", r.Checked))
	}
	return problems
}

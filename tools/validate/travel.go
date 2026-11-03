// travel.json holds, for each city outside Berlin a concert is in — in
// Germany or elsewhere in Europe — what the page needs to help the reader get
// there from Berlin: roughly how long the quickest all-train route takes, or
// the fact that Rome2Rio listed none; and, for a German city, its railway
// station, from which the page builds a bahn.de search for Deutschlandticket
// connections. Nothing in seen.json refers to it.
//
// Unlike artists.json and favorites.json this file IS written by the
// concert-watch routine (see "Train times from Berlin" in CLAUDE.md), and each
// half of an entry is copied from one named source, never estimated:
//
//   - train: the quickest all-train route the Rome2Rio connector returned
//     from Berlin Hbf — its name, duration and operators — with the query that
//     was sent and the day, so a person can repeat it. The page reads the
//     operators to say whether the route is regional only, and so covered by a
//     Deutschlandticket; that verdict is derived there, never stored here.
//   - no_train: in place of train, when none of the routes Rome2Rio returned
//     is all-train: the query, the day, and the names of every route it did
//     return, so the page can say what it listed instead. That is a record of
//     what one answer held, not proof that no rail connection exists —
//     Rome2Rio returns only its top four routes.
//   - station: a German city's station name and DB station number, found with
//     -find-station in tools/stations/de.csv. CI checks the pair against that
//     same file, so a number recalled rather than looked up fails the build.
//
// Any half may be missing — the connector may be unavailable to a run, or a
// place may have no station — but not all of them, and train and no_train
// never appear together. What else can be checked is that a train route is
// made of trains only, rather than the drive, the flight or the train-then-bus
// Rome2Rio lists beside it; that a no_train entry lists no route that is; and
// that the entry is for a city some concert in the dataset is in, so a
// misspelt city, which the page could never match to a card, fails the build
// instead of sitting unused.
package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/stations"
)

// maxTravelMinutes bounds a plausible duration. The longest rail journeys the
// dataset needs, Berlin to central Turkey, take under two days, so a figure
// past three is a unit mix-up (seconds, or hours and minutes run together),
// not a slow connection.
const maxTravelMinutes = 3 * 24 * 60

// TravelTime is one entry in travel.json.
type TravelTime struct {
	// City is the concert's city exactly as seen.json spells it; the page
	// joins on it.
	City    string            `json:"city"`
	Station *stations.Station `json:"station"`
	Train   *TrainRoute       `json:"train"`
	NoTrain *NoTrainRoute     `json:"no_train"`
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

// NoTrainRoute records a Rome2Rio answer that held no all-train route.
type NoTrainRoute struct {
	// Query is the destination sent to Rome2Rio, as for TrainRoute.
	Query string `json:"query"`
	// Routes are the names of every route Rome2Rio returned, copied as
	// returned and in order: "Fly", "Bus, car ferry", "Train, bus".
	Routes []string `json:"routes"`
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

		if e.Station == nil && e.Train == nil && e.NoTrain == nil {
			problems = append(problems, fmt.Sprintf(
				"%s: has no station, train or no_train; leave the city out until a run finds one", label))
		}
		if e.Train != nil && e.NoTrain != nil {
			problems = append(problems, fmt.Sprintf(
				"%s: has both train and no_train; a refreshed entry keeps only the newer answer", label))
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
		if e.NoTrain != nil {
			for _, m := range checkNoTrain(*e.NoTrain) {
				problems = append(problems, fmt.Sprintf("%s: no_train: %s", label, m))
			}
		}
	}

	return problems
}

// Rome2Rio names a route after its legs, joined by commas — "Train",
// "Train via Wolfsburg", "Train, night train", "Train, bus, car ferry",
// "Train to Leipzig-Halle Airport, fly" — and a "via" list is joined the same
// way ("Train via Wolfsburg, Hauptbahnhof"). railModes are the legs a trip
// "by train" may consist of; otherModes are the others it is known to use.
var (
	railModes  = []string{"train", "night train"}
	otherModes = []string{
		"fly", "bus", "drive", "car ferry", "ferry", "taxi", "rideshare", "walk", "eurotunnel",
		"shuttle", "tram", "subway", "metro", "cable car", "funicular", "helicopter", "coach", "town car",
	}
)

func startsWithMode(leg string, modes []string) bool {
	leg = strings.ToLower(leg)
	for _, m := range modes {
		if leg == m || strings.HasPrefix(leg, m+" ") {
			return true
		}
	}
	return false
}

// isRailRoute reports whether every leg of a route Rome2Rio named is a train
// or a night train. A comma segment that names no mode at all is accepted
// only once a "via" has opened a list of places; anywhere else it is an
// unfamiliar mode, and the route is not counted as all-train, which is the
// safe way to be wrong.
func isRailRoute(name string) bool {
	legs := strings.Split(name, ",")
	via := false
	for i, leg := range legs {
		leg = strings.TrimSpace(leg)
		switch {
		case startsWithMode(leg, railModes):
		case i == 0, startsWithMode(leg, otherModes), !via:
			return false
		}
		if strings.Contains(strings.ToLower(leg), " via ") {
			via = true
		}
	}
	return true
}

func checkNoTrain(r NoTrainRoute) []string {
	var problems []string
	for _, req := range []struct{ name, val string }{{"query", r.Query}, {"checked", r.Checked}} {
		if strings.TrimSpace(req.val) == "" {
			problems = append(problems, fmt.Sprintf("field %q is required but empty", req.name))
		}
	}
	// Rome2Rio always returns something — a flight, a drive — so an empty
	// list means the field was dropped, not that nothing came back.
	if len(r.Routes) == 0 {
		problems = append(problems, `field "routes" is required (the names of the routes Rome2Rio returned)`)
	}
	for j, name := range r.Routes {
		switch {
		case strings.TrimSpace(name) == "":
			problems = append(problems, fmt.Sprintf("routes[%d] is empty", j))
		case isRailRoute(name):
			problems = append(problems, fmt.Sprintf(
				"routes lists %q, which is all-train; record the quickest such route as train instead", name))
		}
	}
	if r.Checked != "" && !validDate(r.Checked) {
		problems = append(problems, fmt.Sprintf("checked %q is not a valid YYYY-MM-DD date", r.Checked))
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

	// A route that is not trains all the way — a drive, a flight, or a train
	// to the airport or the coach station — was copied from the wrong line.
	if r.Route != "" && !isRailRoute(r.Route) {
		problems = append(problems, fmt.Sprintf(
			"route %q is not an all-train route (every leg must be a train or night train)", r.Route))
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

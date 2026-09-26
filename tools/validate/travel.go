// travel.json holds, for each German city a concert is in, roughly how long
// the quickest train from Berlin takes to get there. The page prints it on
// those cards; nothing in seen.json refers to it.
//
// Unlike artists.json and favorites.json this file IS written by the
// concert-watch routine: when a concert turns up in a German city the file
// doesn't cover yet, the run asks the Rome2Rio connector for routes from
// Berlin Hbf and copies the quickest train route's name and duration in (see
// "Train times from Berlin" in CLAUDE.md). A duration is a figure a tool
// returned, never one estimated from a map or from memory, so an entry
// records what was asked and what came back — enough for a person to repeat
// the query and check it.
//
// What can be checked here is the shape of an entry, that the route it
// records is a train route rather than the drive or the flight Rome2Rio lists
// beside it, and that it is for a city the dataset actually has a German
// concert in — so a misspelt city, which the page could never match to a
// card, fails the build instead of sitting unused.
package main

import (
	"fmt"
	"strings"
)

// maxTravelMinutes bounds a plausible duration. Nothing in Germany is a day's
// train ride from Berlin, so a larger figure is a unit mix-up (seconds, or
// hours and minutes run together), not a slow connection.
const maxTravelMinutes = 24 * 60

// TravelTime is one entry in travel.json.
type TravelTime struct {
	// City is the concert's city exactly as seen.json spells it; the page
	// joins on it.
	City string `json:"city"`
	// Query is the destination sent to Rome2Rio, which may be fuller than
	// City ("Frankfurt am Main, Germany" for "Frankfurt").
	Query string `json:"query"`
	// Minutes is the duration Rome2Rio gave for Route.
	Minutes int `json:"minutes"`
	// Route is Rome2Rio's own name for the route, copied as returned:
	// "Train", "Train via Wolfsburg", "Train, bus".
	Route string `json:"route"`
	// Checked is the day the query was run.
	Checked string `json:"checked"`
}

// Travel is the top-level shape of travel.json.
type Travel struct {
	Cities []TravelTime `json:"cities"`
}

// ValidateTravel checks travel.json in isolation and against seen.json. An
// empty list is fine: it is the state before any run has filled it in.
func ValidateTravel(t Travel, f File) []string {
	var problems []string

	germanCities := map[string]bool{}
	for _, c := range f.Concerts {
		if c.LocationTag == "germany" {
			germanCities[c.City] = true
		}
	}

	cities := make(map[string]int, len(t.Cities))
	for i, e := range t.Cities {
		label := e.City
		if strings.TrimSpace(label) == "" {
			label = fmt.Sprintf("cities[%d]", i)
		}

		for _, req := range []struct{ name, val string }{
			{"city", e.City}, {"query", e.Query}, {"route", e.Route}, {"checked", e.Checked},
		} {
			if strings.TrimSpace(req.val) == "" {
				problems = append(problems, fmt.Sprintf("%s: field %q is required but empty", label, req.name))
			}
		}

		if e.City != "" {
			if prev, dup := cities[e.City]; dup {
				problems = append(problems, fmt.Sprintf("%s: duplicate city (also cities[%d])", label, prev))
			} else {
				cities[e.City] = i
			}
			if !germanCities[e.City] {
				problems = append(problems, fmt.Sprintf(
					"%s: no concert tagged germany in seen.json has this city; the name must match a row's city exactly", label))
			}
		}

		if e.Minutes < 1 || e.Minutes > maxTravelMinutes {
			problems = append(problems, fmt.Sprintf(
				"%s: minutes %d is outside 1..%d", label, e.Minutes, maxTravelMinutes))
		}

		// Rome2Rio names every option by its first mode — "Drive", "Fly to
		// …", "Bus", "Night train", "Train via …" — so a route that doesn't
		// open with "Train" is one of the others, copied from the wrong line.
		if e.Route != "" && !strings.HasPrefix(e.Route, "Train") {
			problems = append(problems, fmt.Sprintf(
				"%s: route %q is not a train route (Rome2Rio names those \"Train …\")", label, e.Route))
		}

		if e.Checked != "" && !validDate(e.Checked) {
			problems = append(problems, fmt.Sprintf("%s: checked %q is not a valid YYYY-MM-DD date", label, e.Checked))
		}
	}

	return problems
}

// travel.json holds, for each city outside Berlin a concert is in — in
// Germany or elsewhere in Europe — what the page needs to help the reader get
// there from Berlin: how long the best train-based route takes, or, when
// Rome2Rio listed none, the routes it listed instead; and, for a German city,
// its railway station, from which the page builds a bahn.de search for
// Deutschlandticket connections. Nothing in seen.json refers to it.
//
// Unlike artists.json and favorites.json this file IS written by the
// concert-watch routine (see "Train times from Berlin" in CLAUDE.md), and each
// half of an entry is copied from one named source, never estimated:
//
//   - train: a train-based route the Rome2Rio connector returned from Berlin
//     Hbf — the quickest all-train one, or failing that the quickest that is
//     mostly train (see isTrainBased) — its name, duration and operators, with
//     the query that was sent and the day, so a person can repeat it. The page
//     reads the operators to say whether the route is regional only, and so
//     covered by a Deutschlandticket; that verdict is derived there, never
//     stored here. With it go others, the other routes that same answer
//     listed, so that when the train takes most of a day the page can offer
//     the quickest one that needs no driving — usually a flight. Which one
//     that is, and whether to show it, is likewise derived there.
//   - no_train: in place of train, when none of the routes Rome2Rio returned
//     is train-based: the query, the day, and every route it did return, from
//     which the page picks the quickest one that needs no driving. That is a
//     record of what one answer held, not proof that no rail connection
//     exists — Rome2Rio returns only its top four routes.
//   - station: a German city's station name and DB station number, found with
//     -find-station in tools/stations/de.csv. CI checks the pair against that
//     same file, so a number recalled rather than looked up fails the build.
//
// Any half may be missing — the connector may be unavailable to a run, or a
// place may have no station — but not all of them, and train and no_train
// never appear together. What else can be checked is that a train route is
// train-based rather than the drive or the flight Rome2Rio lists beside it,
// and the best such route among those recorded with it; that a long train
// route carries the others at all; that a no_train entry lists no route that
// is train-based; and that the entry is for a
// city some concert in the dataset is in, so a misspelt city, which the page could never
// match to a card, fails the build instead of sitting unused.
package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/stations"
)

// Bounds on a plausible duration; past them a figure is a unit mix-up
// (seconds read as minutes), not a slow connection. Nothing in Germany is a
// day's train ride from Berlin. Abroad, a train-based route can take days —
// the one Rome2Rio listed to Yerevan on 2026-11-07 took more than three — and
// so can a route listed beside the train ones (the bus and car ferry to the
// Canary Islands take four), but none takes a week.
const (
	maxGermanTrainMinutes = 24 * 60
	maxTrainMinutes       = 7 * 24 * 60
	maxListedMinutes      = 7 * 24 * 60
)

// longTrainMinutes is where a train-based route stops being the obvious way
// to go: past eight hours the page also offers the quickest route that needs
// no driving, if one is clearly quicker, and it can only pick that from the
// other routes the same answer listed. So past this a train must carry them.
// Below it they are optional, which keeps entries written before the field
// existed valid. index.html's LONG_TRAIN_MINUTES is the same number.
const longTrainMinutes = 8 * 60

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
	// Others are the other routes the same answer listed, copied as returned
	// and in order. Nil when they were not recorded, which only a route of
	// up to longTrainMinutes may leave; empty when the answer listed nothing
	// else.
	Others []ListedRoute `json:"others"`
	// Checked is the day the query was run.
	Checked string `json:"checked"`
}

// NoTrainRoute records a Rome2Rio answer that held no train-based route.
type NoTrainRoute struct {
	// Query is the destination sent to Rome2Rio, as for TrainRoute.
	Query string `json:"query"`
	// Routes are every route Rome2Rio returned, copied as returned and in
	// order.
	Routes []ListedRoute `json:"routes"`
	// Checked is the day the query was run.
	Checked string `json:"checked"`
}

// ListedRoute is one route of a Rome2Rio answer, copied as returned.
type ListedRoute struct {
	// Name is Rome2Rio's name for it: "Fly", "Bus, car ferry", "Drive".
	Name    string `json:"name"`
	Minutes int    `json:"minutes"`
	// Carriers may be empty: Rome2Rio names none for a drive.
	Carriers []string `json:"carriers"`
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
	// with no germany or europe row left is simply not shown.
	//
	// The tags are used only to pick a duration bound, where a retag can only
	// loosen it, or tighten it for a city abroad wrongly tagged germany.
	knownCities := map[string]bool{}
	tags := map[string]map[string]bool{}
	for _, c := range f.Concerts {
		knownCities[c.City] = true
		if tags[c.City] == nil {
			tags[c.City] = map[string]bool{}
		}
		tags[c.City][c.LocationTag] = true
	}
	onlyTagged := func(city string, allowed ...string) bool {
		if len(tags[city]) == 0 {
			return false
		}
		for tag := range tags[city] {
			if !slices.Contains(allowed, tag) {
				return false
			}
		}
		return true
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
			maxMinutes := maxTrainMinutes
			if onlyTagged(e.City, "germany", "berlin") {
				maxMinutes = maxGermanTrainMinutes
			}
			for _, m := range checkTrain(*e.Train, maxMinutes) {
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

// Rome2Rio names a route after its legs, joined by commas: "Train",
// "Train via Wolfsburg", "Train, night train", "Train, bus, car ferry",
// "Fly Berlin Brandenburg Airport to Gran Canaria Airport", "Drive,
// Eurotunnel". A "via" list is joined the same way ("Train via Wolfsburg,
// Hauptbahnhof"), its places capitalized where a later leg is not ("bus").
var (
	// railModes are the legs of a trip by train. A rail leg is the bare word
	// or the word followed by "via", "to" or "from", so "Train and bus" is
	// not one.
	railModes = []string{"train", "night train"}
	// excludedModes rule a route out of being train-based: a reader who
	// cannot drive can't take a drive, and a flight is not a train trip.
	excludedModes = []string{"fly", "drive"}
	// otherModes are the other legs Rome2Rio is known to use.
	otherModes = []string{
		"bus", "car ferry", "ferry", "taxi", "rideshare", "walk", "eurotunnel", "shuttle",
		"tram", "subway", "metro", "cable car", "funicular", "helicopter", "coach", "town car", "towncar",
		"water taxi", "boat",
	}
)

func isRailLeg(leg string) bool {
	for _, m := range railModes {
		if leg == m {
			return true
		}
		for _, rest := range []string{" via ", " to ", " from "} {
			if strings.HasPrefix(leg, m+rest) {
				return true
			}
		}
	}
	return false
}

func hasModePrefix(leg string, modes []string) bool {
	for _, m := range modes {
		if leg == m || strings.HasPrefix(leg, m+" ") {
			return true
		}
	}
	return false
}

// legSummary is what a route's name says about its legs.
type legSummary struct {
	rail, total int
	// excluded: some leg is a flight or a drive.
	excluded bool
	// unknown: some segment is neither a known mode nor, after a "via", a
	// capitalized place. The route is then not train-based, which is the
	// safe way to be wrong.
	unknown bool
}

func readLegs(name string) legSummary {
	var s legSummary
	via := false
	for i, seg := range strings.Split(name, ",") {
		seg = strings.TrimSpace(seg)
		leg := strings.ToLower(seg)
		switch {
		case isRailLeg(leg):
			s.rail++
			s.total++
		case hasModePrefix(leg, excludedModes):
			s.excluded = true
			s.total++
		case hasModePrefix(leg, otherModes):
			s.total++
		case i > 0 && via && startsUpper(seg):
			// A place in the via list, not a leg.
		default:
			s.unknown = true
		}
		if strings.Contains(leg, " via ") {
			via = true
		}
	}
	return s
}

func startsUpper(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

// isTrainBased reports whether a route is mostly train: at least half its
// legs are trains, and none is a flight or a drive. Rome2Rio gives one
// duration for the whole route, not one per leg, so "mostly" is counted in
// legs, not hours — "Train, bus" counts, whether the bus is the last ten
// minutes to a village or a day across the Balkans, and the page names the
// modes so the reader can tell.
func isTrainBased(name string) bool {
	s := readLegs(name)
	return !s.unknown && !s.excluded && s.rail > 0 && 2*s.rail >= s.total
}

// isAllTrain reports whether every leg of a route is a train or a night
// train. Such a route is preferred over a mixed one even when the mixed one is
// quicker (ROUTINE.md, step 6a).
func isAllTrain(name string) bool {
	s := readLegs(name)
	return !s.unknown && !s.excluded && s.rail > 0 && s.rail == s.total
}

// checkProvenance checks the query and day every route answer records.
func checkProvenance(query, checked string) []string {
	var problems []string
	for _, req := range []struct{ name, val string }{{"query", query}, {"checked", checked}} {
		if strings.TrimSpace(req.val) == "" {
			problems = append(problems, fmt.Sprintf("field %q is required but empty", req.name))
		}
	}
	if checked != "" && !validDate(checked) {
		problems = append(problems, fmt.Sprintf("checked %q is not a valid YYYY-MM-DD date", checked))
	}
	return problems
}

func checkCarriers(carriers []string) []string {
	var problems []string
	for j, name := range carriers {
		switch {
		case strings.TrimSpace(name) == "":
			problems = append(problems, fmt.Sprintf("carriers[%d] is empty", j))
		case slices.Contains(carriers[:j], name):
			problems = append(problems, fmt.Sprintf("carriers lists %q twice", name))
		}
	}
	return problems
}

func checkNoTrain(r NoTrainRoute) []string {
	problems := checkProvenance(r.Query, r.Checked)
	// Rome2Rio always returns something — a flight, a drive — so an empty
	// list means the field was dropped, not that nothing came back.
	if len(r.Routes) == 0 {
		problems = append(problems, `field "routes" is required (every route Rome2Rio returned)`)
	}
	for j, route := range r.Routes {
		label, routeProblems := checkListedRoute("routes", j, route)
		problems = append(problems, routeProblems...)
		if isTrainBased(route.Name) {
			problems = append(problems, label+": is train-based; record the best such route as train instead")
		}
	}
	return problems
}

// checkListedRoute checks one route copied from a Rome2Rio answer, and
// returns the label its problems carry so a caller can add its own.
func checkListedRoute(field string, j int, route ListedRoute) (string, []string) {
	var problems []string
	label := fmt.Sprintf("%s[%d]", field, j)
	if strings.TrimSpace(route.Name) == "" {
		problems = append(problems, label+`: field "name" is required but empty`)
	} else {
		label = fmt.Sprintf("%s[%d] %q", field, j, route.Name)
	}
	if route.Minutes < 1 || route.Minutes > maxListedMinutes {
		problems = append(problems, fmt.Sprintf("%s: minutes %d is outside 1..%d", label, route.Minutes, maxListedMinutes))
	}
	for _, m := range checkCarriers(route.Carriers) {
		problems = append(problems, label+": "+m)
	}
	return label, problems
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

func checkTrain(r TrainRoute, maxMinutes int) []string {
	problems := checkProvenance(r.Query, r.Checked)
	if strings.TrimSpace(r.Route) == "" {
		problems = append(problems, `field "route" is required but empty`)
	}

	if r.Minutes < 1 || r.Minutes > maxMinutes {
		problems = append(problems, fmt.Sprintf("minutes %d is outside 1..%d", r.Minutes, maxMinutes))
	}

	// A drive, a flight, a train to the airport, or a route that is mostly
	// bus or ferry was copied from the wrong line.
	if r.Route != "" && !isTrainBased(r.Route) {
		problems = append(problems, fmt.Sprintf(
			"route %q is not train-based (at least half its legs trains, none a flight or a drive)", r.Route))
	}

	// Every train route Rome2Rio returns names who runs it, so an empty list
	// means the field was dropped, not that nobody does.
	if len(r.Carriers) == 0 {
		problems = append(problems, `field "carriers" is required (the operators Rome2Rio listed for the route)`)
	}
	problems = append(problems, checkCarriers(r.Carriers)...)

	if r.Others == nil && r.Minutes > longTrainMinutes {
		problems = append(problems, fmt.Sprintf(
			`field "others" is required when the route takes more than %d minutes (every other route Rome2Rio returned; [] if it returned none)`,
			longTrainMinutes))
	}
	return append(problems, checkOthers(r)...)
}

// checkOthers checks the routes recorded beside a train route, and that none
// of them is the one step 6a would have picked instead: an all-train route
// beats a mixed one whatever their durations, and within a kind the quicker
// wins. That turns "the best train-based route" from a claim into something
// the file itself shows.
func checkOthers(r TrainRoute) []string {
	var problems []string
	chosenAllTrain := isAllTrain(r.Route)
	for j, o := range r.Others {
		label, routeProblems := checkListedRoute("others", j, o)
		problems = append(problems, routeProblems...)
		if o.Name == r.Route && o.Minutes == r.Minutes {
			problems = append(problems, label+": repeats the route itself; list only the other routes")
			continue
		}
		// A route that isn't train-based is reported on its own, and nothing
		// can be said about what should have been chosen in its place.
		if !isTrainBased(r.Route) || !isTrainBased(o.Name) {
			continue
		}
		switch otherAllTrain := isAllTrain(o.Name); {
		case otherAllTrain && !chosenAllTrain:
			problems = append(problems, fmt.Sprintf(
				"%s: is all-train while route %q is not; an all-train route is preferred even when slower", label, r.Route))
		case otherAllTrain == chosenAllTrain && o.Minutes < r.Minutes:
			problems = append(problems, fmt.Sprintf(
				"%s: takes %d minutes, less than route %q at %d; record the quicker one as the route", label, o.Minutes, r.Route, r.Minutes))
		}
	}
	return problems
}

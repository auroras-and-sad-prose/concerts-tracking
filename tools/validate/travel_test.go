package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/stations"
)

// travelEntry is a correct entry for valid()'s city; tests mutate a copy.
func travelEntry() TravelTime {
	return TravelTime{
		City:    "Altenkrempe",
		Station: &stations.Station{Name: "Neustadt (Holst)", EVA: "8004349"},
		Train: &TrainRoute{
			Query:    "Altenkrempe, Germany",
			Minutes:  190,
			Route:    "Train",
			Carriers: []string{"Deutsche Bahn Regio (DB Regional)"},
			Checked:  "2026-09-26",
		},
	}
}

// stationList is a two-row station list holding travelEntry's station.
func stationList(t *testing.T) *stations.List {
	t.Helper()
	path := filepath.Join(t.TempDir(), "de.csv")
	if err := os.WriteFile(path, []byte("name;eva\nNeustadt (Holst);8004349\nKempen (Niederrhein);8000409\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := stations.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestValidTravel(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry()}}, f, stationList(t)); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

// Either half may be missing on its own: the connector may be unavailable to
// a run, or a place may have no station.
func TestTravelEitherHalfAlone(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	onlyStation, onlyTrain := travelEntry(), travelEntry()
	onlyStation.Train = nil
	onlyTrain.Station = nil
	for _, e := range []TravelTime{onlyStation, onlyTrain} {
		if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, f, stationList(t)); len(p) != 0 {
			t.Fatalf("expected %+v to be valid, got %v", e, p)
		}
	}
}

// Before any run has filled it in, the file is an empty list.
func TestEmptyTravelIsValid(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	for _, tr := range []Travel{{}, {Cities: []TravelTime{}}} {
		if p := ValidateTravel(tr, f, stationList(t)); len(p) != 0 {
			t.Fatalf("expected an empty list to be valid, got %v", p)
		}
	}
}

func TestTravelFieldChecks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelTime)
	}{
		{"missing city", func(e *TravelTime) { e.City = "" }},
		{"neither half", func(e *TravelTime) { e.Station, e.Train = nil, nil }},
		{"both train and no_train", func(e *TravelTime) { e.NoTrain = noTrain() }},
		{"missing query", func(e *TravelTime) { e.Train.Query = " " }},
		{"missing route", func(e *TravelTime) { e.Train.Route = "" }},
		{"missing checked", func(e *TravelTime) { e.Train.Checked = "" }},
		{"zero minutes", func(e *TravelTime) { e.Train.Minutes = 0 }},
		{"negative minutes", func(e *TravelTime) { e.Train.Minutes = -5 }},
		{"minutes in seconds", func(e *TravelTime) { e.Train.Minutes = 11400 }},
		{"hours and minutes run together", func(e *TravelTime) { e.Train.Minutes = 4550 }},
		{"bad checked date", func(e *TravelTime) { e.Train.Checked = "2026-9-26" }},
		{"impossible checked date", func(e *TravelTime) { e.Train.Checked = "2026-02-30" }},
		{"drive copied instead of train", func(e *TravelTime) { e.Train.Route = "Drive" }},
		{"flight copied instead of train", func(e *TravelTime) { e.Train.Route = "Fly to Stuttgart Airport" }},
		{"flight then train", func(e *TravelTime) {
			e.Train.Route = "Fly Berlin Brandenburg Airport to Düsseldorf International Airport, train"
		}},
		{"bus", func(e *TravelTime) { e.Train.Route = "Bus" }},
		// Each starts with "Train", and none is mostly a trip by train.
		{"train to the airport, then fly", func(e *TravelTime) {
			e.Train.Route = "Train to Leipzig-Halle Airport, fly to Gran Canaria Airport"
		}},
		{"train, bus and ferry", func(e *TravelTime) { e.Train.Route = "Train, bus, car ferry" }},
		{"unknown leg with no via", func(e *TravelTime) { e.Train.Route = "Train, hovercraft" }},
		{"unknown lowercase leg after a via", func(e *TravelTime) { e.Train.Route = "Train via Calais, hovercraft" }},
		{"water taxi after a via", func(e *TravelTime) { e.Train.Route = "Train via Calais, water taxi, bus" }},
		{"train and bus as one leg", func(e *TravelTime) { e.Train.Route = "Train and bus" }},
		{"mostly bus", func(e *TravelTime) { e.Train.Route = "Bus, train, bus" }},
		{"train then a drive", func(e *TravelTime) { e.Train.Route = "Train, drive" }},
		{"German city a day away", func(e *TravelTime) { e.Train.Minutes = 1500 }},
		{"city with no concert", func(e *TravelTime) { e.City = "Altenkremp" }},
		{"no carriers", func(e *TravelTime) { e.Train.Carriers = nil }},
		{"empty carriers", func(e *TravelTime) { e.Train.Carriers = []string{} }},
		{"blank carrier", func(e *TravelTime) { e.Train.Carriers = []string{"Deutsche Bahn Regio (DB Regional)", " "} }},
		{"repeated carrier", func(e *TravelTime) { e.Train.Carriers = []string{"enno", "enno"} }},
		{"missing station name", func(e *TravelTime) { e.Station.Name = "" }},
		{"station number not digits", func(e *TravelTime) { e.Station.EVA = "80043x9" }},
		{"station number too long", func(e *TravelTime) { e.Station.EVA = "80043490" }},
		{"station number recalled wrong", func(e *TravelTime) { e.Station.EVA = "8004350" }},
		{"station not in list", func(e *TravelTime) { e.Station.Name = "Neustadt in Holstein" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := travelEntry()
			tt.mutate(&e)
			f := File{Concerts: []Concert{valid()}}
			if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, f, stationList(t)); len(p) == 0 {
				t.Fatalf("expected a problem for %q, got none", tt.name)
			}
		})
	}
}

func TestTravelAcceptsTrainRouteNames(t *testing.T) {
	for _, route := range []string{
		"Train", "Train via Wolfsburg", "Train via Wolfsburg, Hauptbahnhof", "Train via Frankfurt am Main",
		"Night train", "Train, night train", "Night train, train", "Train to Hamburg Airport",
		// Mostly train: at least half the legs, none a flight or a drive.
		"Train, bus", "Train, bus via Frankfurt am Main", "Train via Hamburg, Lübeck, bus",
		"Train, bus, night train, taxi", "Bus, train", "Train, car ferry", "Night train, bus, train",
	} {
		e := travelEntry()
		e.Train.Route = route
		f := File{Concerts: []Concert{valid()}}
		if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, f, stationList(t)); len(p) != 0 {
			t.Fatalf("route %q should be accepted, got %v", route, p)
		}
	}
}

// noTrain is a correct no_train half, after what Rome2Rio returned for the
// Canary Islands: none of it train-based, a drive with no carriers.
func noTrain() *NoTrainRoute {
	return &NoTrainRoute{
		Query: "Tenerife, Spain",
		Routes: []ListedRoute{
			{Name: "Fly from Berlin Brandenburg Airport", Minutes: 470, Carriers: []string{"easyJet"}},
			{Name: "Train, bus, car ferry", Minutes: 4635, Carriers: []string{"Deutsche Bahn Intercity (DB IC)", "Socibus"}},
			{Name: "Bus, car ferry", Minutes: 4649, Carriers: []string{"FlixBus", "Fred Olsen"}},
			{Name: "Drive", Minutes: 4100, Carriers: []string{}},
		},
		Checked: "2026-11-03",
	}
}

// A city abroad has no station in the German list, and a no_train entry may
// stand alone.
func TestTravelNoTrainAlone(t *testing.T) {
	e := TravelTime{City: "Altenkrempe", NoTrain: noTrain()}
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{valid()}}, stationList(t)); len(p) != 0 {
		t.Fatalf("expected a lone no_train to be valid, got %v", p)
	}
}

func TestTravelNoTrainChecks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NoTrainRoute)
	}{
		{"missing query", func(n *NoTrainRoute) { n.Query = "" }},
		{"missing checked", func(n *NoTrainRoute) { n.Checked = "" }},
		{"bad checked date", func(n *NoTrainRoute) { n.Checked = "2026-11-31" }},
		{"no routes", func(n *NoTrainRoute) { n.Routes = nil }},
		{"blank route name", func(n *NoTrainRoute) { n.Routes[0].Name = " " }},
		{"zero minutes", func(n *NoTrainRoute) { n.Routes[0].Minutes = 0 }},
		{"minutes in seconds", func(n *NoTrainRoute) { n.Routes[1].Minutes = 4635 * 60 }},
		{"blank carrier", func(n *NoTrainRoute) { n.Routes[0].Carriers = []string{" "} }},
		{"repeated carrier", func(n *NoTrainRoute) { n.Routes[0].Carriers = []string{"easyJet", "easyJet"} }},
		// Rome2Rio did list a train-based route, so it belongs in train.
		{"lists a train route", func(n *NoTrainRoute) { n.Routes[0].Name = "Train via Hamburg" }},
		{"lists a night train", func(n *NoTrainRoute) { n.Routes[0].Name = "Night train, train" }},
		{"lists a train-then-bus route", func(n *NoTrainRoute) { n.Routes[0].Name = "Train, bus via Bordeaux" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := TravelTime{City: "Altenkrempe", NoTrain: noTrain()}
			tt.mutate(e.NoTrain)
			if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{valid()}}, stationList(t)); len(p) == 0 {
				t.Fatalf("expected a problem for %q, got none", tt.name)
			}
		})
	}
}

// A city abroad may take up to a week; a German one may not take a day.
func TestTravelDurationBoundByCountry(t *testing.T) {
	abroad := valid()
	abroad.LocationTag = "europe"
	e := travelEntry()
	e.Train.Others = []ListedRoute{} // a route this long must say what else was listed
	e.Train.Minutes = 2728
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{abroad}}, stationList(t)); len(p) != 0 {
		t.Fatalf("a 2728-minute route abroad should pass, got %v", p)
	}
	e.Train.Minutes = 5148
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{abroad}}, stationList(t)); len(p) != 0 {
		t.Fatalf("a 5148-minute route abroad should pass, got %v", p)
	}
	e.Train.Minutes = 10081
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{abroad}}, stationList(t)); len(p) == 0 {
		t.Fatal("a route over a week should fail even abroad")
	}
}

// longTrain is a correct entry for a city most of two days away by train,
// after what Rome2Rio returned for Istanbul on 2026-11-09: the all-train route
// is the one recorded, though a mixed one and a flight are quicker.
func longTrain() TravelTime {
	return TravelTime{
		City: "Altenkrempe",
		Train: &TrainRoute{
			Query:    "Istanbul, Turkey",
			Minutes:  2289,
			Route:    "Train, night train",
			Carriers: []string{"Danish Railways (DSB)", "ÖBB", "Bulgarian Railways (BDZh)"},
			Others: []ListedRoute{
				{Name: "Fly to Istanbul Airport", Minutes: 391, Carriers: []string{"Turkish Airlines"}},
				{Name: "Train, bus", Minutes: 1943, Carriers: []string{"Deutsche Bahn Intercity (DB IC)", "Alpar Turizm"}},
				{Name: "Night train, train", Minutes: 2406, Carriers: []string{"Nightjet", "ÖBB"}},
				{Name: "Drive", Minutes: 1506, Carriers: []string{}},
			},
			Checked: "2026-11-09",
		},
	}
}

// abroadFile is a dataset whose one concert is abroad, so its city may be
// more than a day away.
func abroadFile() File {
	c := valid()
	c.LocationTag = "europe"
	return File{Concerts: []Concert{c}}
}

func TestTravelOthersAccepted(t *testing.T) {
	if p := ValidateTravel(Travel{Cities: []TravelTime{longTrain()}}, abroadFile(), stationList(t)); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}

	// The answer may have listed nothing else.
	alone := longTrain()
	alone.Train.Others = []ListedRoute{}
	if p := ValidateTravel(Travel{Cities: []TravelTime{alone}}, abroadFile(), stationList(t)); len(p) != 0 {
		t.Fatalf("an empty others list should pass, got %v", p)
	}

	// A short route may carry them too; the page just doesn't use them.
	short := travelEntry()
	short.Train.Others = []ListedRoute{{Name: "Drive", Minutes: 170, Carriers: []string{}}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{short}}, File{Concerts: []Concert{valid()}}, stationList(t)); len(p) != 0 {
		t.Fatalf("others on a short route should pass, got %v", p)
	}
}

// Past eight hours the page offers a quicker route beside the train, which it
// can only take from others. Up to eight, entries written before the field
// existed stay valid.
func TestTravelOthersRequiredForLongTrain(t *testing.T) {
	e := longTrain()
	e.Train.Others = nil
	e.Train.Minutes = longTrainMinutes
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, abroadFile(), stationList(t)); len(p) != 0 {
		t.Fatalf("a route of exactly eight hours needs no others, got %v", p)
	}
	e.Train.Minutes = longTrainMinutes + 1
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, abroadFile(), stationList(t)); len(p) == 0 {
		t.Fatal("a route over eight hours without others should be rejected")
	}
}

// Absent and empty are different answers, and the file has to be able to say
// either: "not recorded" fails on a long route, "nothing else listed" passes.
func TestTravelOthersAbsentIsNotEmpty(t *testing.T) {
	for _, tt := range []struct {
		name, others string
		wantProblem  bool
	}{
		{"absent", ``, true},
		{"null", `"others":null,`, true},
		{"empty", `"others":[],`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "travel.json")
			content := `{"cities":[{"city":"Altenkrempe","train":{"query":"Istanbul, Turkey","minutes":2289,"route":"Train, night train","carriers":["ÖBB"],` +
				tt.others + `"checked":"2026-11-09"}}]}`
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write temp file: %v", err)
			}
			travel, err := loadJSON[Travel](path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			p := ValidateTravel(travel, abroadFile(), stationList(t))
			if got := len(p) != 0; got != tt.wantProblem {
				t.Fatalf("problems = %v, want a problem: %v", p, tt.wantProblem)
			}
		})
	}
}

func TestTravelOthersChecks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TrainRoute)
	}{
		{"blank name", func(r *TrainRoute) { r.Others[0].Name = " " }},
		{"zero minutes", func(r *TrainRoute) { r.Others[0].Minutes = 0 }},
		{"minutes in seconds", func(r *TrainRoute) { r.Others[0].Minutes = 391 * 60 * 60 }},
		{"blank carrier", func(r *TrainRoute) { r.Others[0].Carriers = []string{""} }},
		{"repeated carrier", func(r *TrainRoute) { r.Others[2].Carriers = []string{"ÖBB", "ÖBB"} }},
		{"repeats the route itself", func(r *TrainRoute) {
			r.Others[2] = ListedRoute{Name: r.Route, Minutes: r.Minutes, Carriers: r.Carriers}
		}},
		// Step 6a takes the quickest all-train route, so a quicker one listed
		// beside the route means the wrong line was copied.
		{"a quicker all-train route", func(r *TrainRoute) { r.Others[2].Minutes = 2200 }},
		// It prefers all-train to mixed whatever their durations.
		{"an all-train route beside a mixed one", func(r *TrainRoute) {
			r.Route, r.Minutes = "Train, bus", 1943
			r.Others[1] = ListedRoute{Name: "Train, night train", Minutes: 2289, Carriers: []string{"ÖBB"}}
		}},
		// With no all-train route, the quickest mixed one.
		{"a quicker mixed route beside a mixed one", func(r *TrainRoute) {
			r.Route, r.Minutes = "Train, bus, night train, taxi", 2782
			r.Others = []ListedRoute{{Name: "Train, bus", Minutes: 2500, Carriers: []string{"Alpar Turizm"}}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := longTrain()
			tt.mutate(e.Train)
			if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, abroadFile(), stationList(t)); len(p) == 0 {
				t.Fatalf("expected a problem for %q, got none", tt.name)
			}
		})
	}
}

// The best-route check is about what step 6a picks, not about the page, so it
// holds on a short train too, and on a German one under the day's cap.
func TestTravelOthersCheckedOnShortTrain(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	ok := travelEntry()
	ok.Train.Others = []ListedRoute{
		{Name: "Train", Minutes: 240, Carriers: []string{"Deutsche Bahn Intercity (DB IC)"}},
		{Name: "Bus", Minutes: 150, Carriers: []string{"FlixBus"}},
	}
	if p := ValidateTravel(Travel{Cities: []TravelTime{ok}}, f, stationList(t)); len(p) != 0 {
		t.Fatalf("slower and non-train others should pass, got %v", p)
	}

	quicker := travelEntry()
	quicker.Train.Others = []ListedRoute{{Name: "Train", Minutes: 170, Carriers: []string{"Deutsche Bahn Intercity (DB IC)"}}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{quicker}}, f, stationList(t)); len(p) == 0 {
		t.Fatal("a quicker all-train route beside a short train should be rejected")
	}

	tooLong := travelEntry()
	tooLong.Train.Others = []ListedRoute{{Name: "Bus", Minutes: maxListedMinutes + 1, Carriers: []string{"FlixBus"}}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{tooLong}}, f, stationList(t)); len(p) == 0 {
		t.Fatal("an other route over a week should be rejected")
	}
}

// index.html offers a quicker route past the same eight hours after which
// this requires others; the shared fixture keeps the two numbers together.
func TestLongTrainMatchesFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "long-train.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		LongTrainMinutes int `json:"long_train_minutes"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if fixture.LongTrainMinutes != longTrainMinutes {
		t.Fatalf("longTrainMinutes = %d, fixture says %d; change index.html's LONG_TRAIN_MINUTES with it",
			longTrainMinutes, fixture.LongTrainMinutes)
	}
}

// index.html reads route names too, to label a route and to tell a
// train-based one from the rest; the shared fixture holds its reading and
// this one to the same answers.
func TestRouteReadingMatchesFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "routes.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		Routes []struct {
			Name       string `json:"name"`
			TrainBased bool   `json:"train_based"`
			AllTrain   bool   `json:"all_train"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fixture.Routes) == 0 {
		t.Fatal("fixture lists no routes")
	}
	// Every mode this reader knows needs a case, so that a mode added here
	// alone meets index.html's reader in the smoke suite.
	for _, mode := range slices.Concat(railModes, excludedModes, otherModes) {
		covered := slices.ContainsFunc(fixture.Routes, func(r struct {
			Name       string `json:"name"`
			TrainBased bool   `json:"train_based"`
			AllTrain   bool   `json:"all_train"`
		}) bool {
			for _, seg := range strings.Split(r.Name, ",") {
				if leg := strings.ToLower(strings.TrimSpace(seg)); leg == mode || strings.HasPrefix(leg, mode+" ") {
					return true
				}
			}
			return false
		})
		if !covered {
			t.Errorf("mode %q has no case in the fixture; add one, and the mode to index.html", mode)
		}
	}
	for _, r := range fixture.Routes {
		if got := isTrainBased(r.Name); got != r.TrainBased {
			t.Errorf("isTrainBased(%q) = %v, fixture says %v", r.Name, got, r.TrainBased)
		}
		if got := isAllTrain(r.Name); got != r.AllTrain {
			t.Errorf("isAllTrain(%q) = %v, fixture says %v", r.Name, got, r.AllTrain)
		}
	}
}

func TestTravelDuplicateCity(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry(), travelEntry()}}, f, stationList(t)); len(p) == 0 {
		t.Fatal("expected a problem for a city listed twice")
	}
}

// location_tag is refinable and travel.json entries are never removed, so an
// entry must stay valid whatever its city's rows are tagged; one retagged to
// berlin just stops being shown.
func TestTravelCitySurvivesRetag(t *testing.T) {
	for _, tag := range []string{"berlin", "europe"} {
		c := valid()
		c.LocationTag = tag
		if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry()}}, File{Concerts: []Concert{c}}, stationList(t)); len(p) != 0 {
			t.Fatalf("entry should survive a retag to %s, got %v", tag, p)
		}
	}
}

// An entry may be added, refreshed or have its answer replaced, but not
// dropped: the routine never removes one.
func TestTravelAppendOnly(t *testing.T) {
	kempen := travelEntry()
	kempen.City = "Kempen"
	base := Travel{Cities: []TravelTime{travelEntry(), kempen}}

	refreshed := travelEntry()
	refreshed.Station = nil
	refreshed.Train = nil
	refreshed.NoTrain = &NoTrainRoute{
		Query:   "Altenkrempe, Germany",
		Routes:  []ListedRoute{{Name: "Drive", Minutes: 180}},
		Checked: "2026-11-01",
	}
	added := travelEntry()
	added.City = "Rostock"
	head := Travel{Cities: []TravelTime{added, kempen, refreshed}}
	if p := CheckTravelAppendOnly(base, head); len(p) != 0 {
		t.Fatalf("expected a reordered, refreshed and extended file to pass, got %v", p)
	}
	if p := CheckTravelAppendOnly(Travel{}, head); len(p) != 0 {
		t.Fatalf("expected an empty base to allow anything, got %v", p)
	}

	p := CheckTravelAppendOnly(base, Travel{Cities: []TravelTime{kempen}})
	if len(p) != 1 || !strings.Contains(p[0], `"Altenkrempe" was removed`) {
		t.Fatalf("expected one problem naming the removed city, got %v", p)
	}
	// A respelt city is a removal plus an addition; the page joins on the name.
	respelt := travelEntry()
	respelt.City = "altenkrempe"
	if p := CheckTravelAppendOnly(base, Travel{Cities: []TravelTime{respelt, kempen}}); len(p) != 1 {
		t.Fatalf("expected a respelt city to count as removed, got %v", p)
	}
}

// The base is read for its cities alone, so a reviewed change to the file's
// shape isn't failed by the old shape still sitting on main.
func TestTravelBaseToleratesAnotherSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "travel.json")
	old := `{"version": 1, "cities": [{"city": "Kempen", "train": "5h", "retired_field": true}, {"city": "Rostock"}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadJSON[Travel](path); err == nil {
		t.Fatal("expected the strict loader to reject this file; the test no longer shows anything")
	}
	base, err := loadTravelBase(path)
	if err != nil {
		t.Fatalf("expected the base to load, got %v", err)
	}
	kempen := travelEntry()
	kempen.City = "Kempen"
	p := CheckTravelAppendOnly(base, Travel{Cities: []TravelTime{kempen}})
	if len(p) != 1 || !strings.Contains(p[0], `"Rostock" was removed`) {
		t.Fatalf("expected one problem naming Rostock, got %v", p)
	}

	if err := os.WriteFile(path, []byte(`{"cities": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadTravelBase(path); err == nil {
		t.Fatal("expected a base that is not JSON to be an error")
	}
}

func TestTravelRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "travel.json")
	content := `{"cities":[{"city":"Altenkrempe","train":{"query":"Altenkrempe, Germany","minutes":190,"route":"Train","carriers":["enno"],"checked":"2026-09-26","via":"Lübeck"}}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := loadJSON[Travel](path); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

// -find-station prints an answer the routine can copy verbatim, and says so
// plainly when there is nothing to copy.
func TestPrintStation(t *testing.T) {
	l := stationList(t)
	for _, tt := range []struct{ query, want string }{
		{"Kempen", `Kempen (qualified name): {"name":"Kempen (Niederrhein)","eva":"8000409"}`},
		{"Altenkrempe", "Altenkrempe: no station found"},
	} {
		var b strings.Builder
		printStation(&b, tt.query, l.Find(tt.query))
		if !strings.HasPrefix(b.String(), tt.want) {
			t.Errorf("printStation(%q) = %q, want prefix %q", tt.query, b.String(), tt.want)
		}
	}
}

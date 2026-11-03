package main

import (
	"os"
	"path/filepath"
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

// A city abroad may take up to three days; a German one may not take one.
func TestTravelDurationBoundByCountry(t *testing.T) {
	abroad := valid()
	abroad.LocationTag = "europe"
	e := travelEntry()
	e.Train.Minutes = 2728
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{abroad}}, stationList(t)); len(p) != 0 {
		t.Fatalf("a 2728-minute route abroad should pass, got %v", p)
	}
	e.Train.Minutes = 4321
	if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, File{Concerts: []Concert{abroad}}, stationList(t)); len(p) == 0 {
		t.Fatal("a route over three days should fail even abroad")
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

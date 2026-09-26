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
			Route:    "Train, bus",
			Carriers: []string{"Deutsche Bahn Regio (DB Regional)", "Autokraft"},
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
		{"missing query", func(e *TravelTime) { e.Train.Query = " " }},
		{"missing route", func(e *TravelTime) { e.Train.Route = "" }},
		{"missing checked", func(e *TravelTime) { e.Train.Checked = "" }},
		{"zero minutes", func(e *TravelTime) { e.Train.Minutes = 0 }},
		{"negative minutes", func(e *TravelTime) { e.Train.Minutes = -5 }},
		{"minutes in seconds", func(e *TravelTime) { e.Train.Minutes = 11400 }},
		{"bad checked date", func(e *TravelTime) { e.Train.Checked = "2026-9-26" }},
		{"impossible checked date", func(e *TravelTime) { e.Train.Checked = "2026-02-30" }},
		{"drive copied instead of train", func(e *TravelTime) { e.Train.Route = "Drive" }},
		{"flight copied instead of train", func(e *TravelTime) { e.Train.Route = "Fly to Stuttgart Airport" }},
		{"flight then train", func(e *TravelTime) {
			e.Train.Route = "Fly Berlin Brandenburg Airport to Düsseldorf International Airport, train"
		}},
		{"night train", func(e *TravelTime) { e.Train.Route = "Night train" }},
		{"bus", func(e *TravelTime) { e.Train.Route = "Bus" }},
		{"city with no German concert", func(e *TravelTime) { e.City = "Altenkremp" }},
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
	for _, route := range []string{"Train", "Train via Wolfsburg", "Train via Wolfsburg, Hauptbahnhof", "Train, bus"} {
		e := travelEntry()
		e.Train.Route = route
		f := File{Concerts: []Concert{valid()}}
		if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, f, stationList(t)); len(p) != 0 {
			t.Fatalf("route %q should be accepted, got %v", route, p)
		}
	}
}

func TestTravelDuplicateCity(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry(), travelEntry()}}, f, stationList(t)); len(p) == 0 {
		t.Fatal("expected a problem for a city listed twice")
	}
}

// The page only shows a time on German cards, so an entry for a city whose
// concerts are all in Berlin or abroad would never be shown.
func TestTravelCityMustBeGerman(t *testing.T) {
	c := valid()
	c.LocationTag = "europe"
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry()}}, File{Concerts: []Concert{c}}, stationList(t)); len(p) == 0 {
		t.Fatal("expected a problem for a city with no concert tagged germany")
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

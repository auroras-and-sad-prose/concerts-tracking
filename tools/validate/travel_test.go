package main

import (
	"os"
	"path/filepath"
	"testing"
)

// travelEntry is a correct entry for valid()'s city; tests mutate a copy.
func travelEntry() TravelTime {
	return TravelTime{
		City:    "Altenkrempe",
		Query:   "Altenkrempe, Germany",
		Minutes: 190,
		Route:   "Train, bus",
		Checked: "2026-09-26",
	}
}

func TestValidTravel(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry()}}, f); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

// Before any run has filled it in, the file is an empty list.
func TestEmptyTravelIsValid(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	for _, tr := range []Travel{{}, {Cities: []TravelTime{}}} {
		if p := ValidateTravel(tr, f); len(p) != 0 {
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
		{"missing query", func(e *TravelTime) { e.Query = " " }},
		{"missing route", func(e *TravelTime) { e.Route = "" }},
		{"missing checked", func(e *TravelTime) { e.Checked = "" }},
		{"zero minutes", func(e *TravelTime) { e.Minutes = 0 }},
		{"negative minutes", func(e *TravelTime) { e.Minutes = -5 }},
		{"minutes in seconds", func(e *TravelTime) { e.Minutes = 11400 }},
		{"bad checked date", func(e *TravelTime) { e.Checked = "2026-9-26" }},
		{"impossible checked date", func(e *TravelTime) { e.Checked = "2026-02-30" }},
		{"drive copied instead of train", func(e *TravelTime) { e.Route = "Drive" }},
		{"flight copied instead of train", func(e *TravelTime) { e.Route = "Fly to Stuttgart Airport" }},
		{"flight then train", func(e *TravelTime) {
			e.Route = "Fly Berlin Brandenburg Airport to Düsseldorf International Airport, train"
		}},
		{"night train", func(e *TravelTime) { e.Route = "Night train" }},
		{"bus", func(e *TravelTime) { e.Route = "Bus" }},
		{"city with no German concert", func(e *TravelTime) { e.City = "Altenkremp" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := travelEntry()
			tt.mutate(&e)
			f := File{Concerts: []Concert{valid()}}
			if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, f); len(p) == 0 {
				t.Fatalf("expected a problem for %q, got none", tt.name)
			}
		})
	}
}

func TestTravelAcceptsTrainRouteNames(t *testing.T) {
	for _, route := range []string{"Train", "Train via Wolfsburg", "Train via Wolfsburg, Hauptbahnhof", "Train, bus"} {
		e := travelEntry()
		e.Route = route
		f := File{Concerts: []Concert{valid()}}
		if p := ValidateTravel(Travel{Cities: []TravelTime{e}}, f); len(p) != 0 {
			t.Fatalf("route %q should be accepted, got %v", route, p)
		}
	}
}

func TestTravelDuplicateCity(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry(), travelEntry()}}, f); len(p) == 0 {
		t.Fatal("expected a problem for a city listed twice")
	}
}

// The page only shows a time on German cards, so an entry for a city whose
// concerts are all in Berlin or abroad would never be shown.
func TestTravelCityMustBeGerman(t *testing.T) {
	c := valid()
	c.LocationTag = "europe"
	if p := ValidateTravel(Travel{Cities: []TravelTime{travelEntry()}}, File{Concerts: []Concert{c}}); len(p) == 0 {
		t.Fatal("expected a problem for a city with no concert tagged germany")
	}
}

func TestTravelRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "travel.json")
	content := `{"cities":[{"city":"Altenkrempe","query":"Altenkrempe, Germany","minutes":190,"route":"Train","checked":"2026-09-26","via":"Lübeck"}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := loadJSON[Travel](path); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

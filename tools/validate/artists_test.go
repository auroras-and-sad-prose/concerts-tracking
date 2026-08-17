package main

import (
	"os"
	"path/filepath"
	"testing"
)

// roster is a correct two-artist roster; tests mutate a copy.
func roster() Artists {
	return Artists{Artists: []Artist{
		{Slug: "scheps", Name: "Olga Scheps", Instruments: []string{"piano"}},
		{Slug: "fischer", Name: "Julia Fischer", Instruments: []string{"violin", "piano"}},
	}}
}

func TestValidRoster(t *testing.T) {
	if p := ValidateArtists(roster()); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

func TestRosterFieldChecks(t *testing.T) {
	tests := []struct {
		name  string
		mutel func(*Artists)
	}{
		{"empty roster", func(a *Artists) { a.Artists = nil }},
		{"missing slug", func(a *Artists) { a.Artists[0].Slug = "" }},
		{"slug not lowercase", func(a *Artists) { a.Artists[0].Slug = "Scheps" }},
		{"missing name", func(a *Artists) { a.Artists[0].Name = "  " }},
		{"no instruments", func(a *Artists) { a.Artists[0].Instruments = nil }},
		{"empty instruments", func(a *Artists) { a.Artists[0].Instruments = []string{} }},
		{"unknown instrument", func(a *Artists) { a.Artists[0].Instruments = []string{"theremin"} }},
		{"blank instrument", func(a *Artists) { a.Artists[0].Instruments = []string{""} }},
		{"repeated instrument", func(a *Artists) { a.Artists[0].Instruments = []string{"piano", "piano"} }},
		{"duplicate slug", func(a *Artists) { a.Artists[1].Slug = "scheps" }},
		{"duplicate name", func(a *Artists) { a.Artists[1].Name = "Olga Scheps" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := roster()
			tt.mutel(&a)
			if p := ValidateArtists(a); len(p) == 0 {
				t.Fatalf("expected a problem for %q, got none", tt.name)
			}
		})
	}
}

// An unknown field in artists.json (say, a hand-edit that misspells
// "instruments") must fail rather than decode to an empty roster entry.
func TestRosterRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artists.json")
	content := `{"artists":[{"slug":"scheps","name":"Olga Scheps","instrument":["piano"]}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := loadJSON[Artists](path); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestRosterMatchesConcerts(t *testing.T) {
	f := File{Concerts: []Concert{valid()}}
	if p := CheckRoster(f, roster()); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

// A concert by someone not on the roster would render with no instrument and
// be invisible to the instrument filter, so it fails the build instead.
func TestUnregisteredArtistRejected(t *testing.T) {
	c := valid()
	c.ID = "perlman|2026-09-12|altenkrempe"
	c.Artist = "Itzhak Perlman"
	if p := CheckRoster(File{Concerts: []Concert{c}}, roster()); len(p) == 0 {
		t.Fatal("expected a problem for an artist missing from the roster")
	}
}

// The row's artist name is what the page joins on, so a name that disagrees
// with the roster (accent dropped, initial added) has to surface too.
func TestArtistNameMismatchRejected(t *testing.T) {
	c := valid()
	c.Artist = "Olga Scheps-Smith"
	if p := CheckRoster(File{Concerts: []Concert{c}}, roster()); len(p) == 0 {
		t.Fatal("expected a problem for an artist name that disagrees with the roster")
	}
}

// A row may narrow the roster to the instrument this engagement calls for:
// Julia Fischer plays both, so either — or both — is a valid claim for one
// concert, while leaving it unstated stays valid too.
func TestConcertInstrumentsMayNarrowRoster(t *testing.T) {
	for _, list := range [][]string{nil, {"piano"}, {"violin"}, {"violin", "piano"}} {
		c := valid()
		c.ID = "fischer|2026-09-12|altenkrempe"
		c.Artist = "Julia Fischer"
		c.Instruments = list
		if p := CheckRoster(File{Concerts: []Concert{c}}, roster()); len(p) != 0 {
			t.Fatalf("instruments %v should be accepted for Julia Fischer, got %v", list, p)
		}
	}
}

// Claiming an instrument the artist isn't recorded as playing means either a
// misread source or a stale roster; both need a human, so the build fails.
func TestConcertInstrumentOutsideRosterRejected(t *testing.T) {
	c := valid() // Olga Scheps, a pianist on this roster
	c.Instruments = []string{"violin"}
	if p := CheckRoster(File{Concerts: []Concert{c}}, roster()); len(p) == 0 {
		t.Fatal("expected a problem for an instrument the artist does not play")
	}
}

// An instrument outside the vocabulary is Validate's complaint; CheckRoster
// must not add a second, redundant one about the same value.
func TestUnknownInstrumentNotDoubleReported(t *testing.T) {
	c := valid()
	c.Instruments = []string{"theremin"}
	if p := CheckRoster(File{Concerts: []Concert{c}}, roster()); len(p) != 0 {
		t.Fatalf("expected vocabulary errors to be left to Validate, got %v", p)
	}
}

// A roster entry with no concerts yet is normal: an artist is added to the
// roster before their first engagement is recorded.
func TestRosterMayHaveUnusedArtists(t *testing.T) {
	if p := CheckRoster(File{Concerts: nil}, roster()); len(p) != 0 {
		t.Fatalf("an artist with no concerts yet should be fine, got %v", p)
	}
}

// Malformed ids are Validate's business; CheckRoster must not pile on a
// second, misleading "unknown artist" complaint about the same row.
func TestMalformedIDSkippedByRosterCheck(t *testing.T) {
	c := valid()
	c.ID = "scheps-altenkrempe"
	if p := CheckRoster(File{Concerts: []Concert{c}}, roster()); len(p) != 0 {
		t.Fatalf("expected malformed ids to be left to Validate, got %v", p)
	}
}

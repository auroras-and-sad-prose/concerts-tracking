package main

import (
	"testing"
	"time"
)

// now is fixed so the year-window check is deterministic in tests.
var now = time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

func strptr(s string) *string { return &s }

// valid is a fully correct concert used as a baseline; tests mutate a copy.
func valid() Concert {
	return Concert{
		ID:          "scheps|2026-09-12|altenkrempe",
		Artist:      "Olga Scheps",
		Date:        "2026-09-12",
		City:        "Altenkrempe",
		Country:     "Germany",
		Venue:       strptr("Kultur Gut Hasselburg"),
		Program:     nil,
		LocationTag: "germany",
		SourceURL:   "https://www.olgascheps.com/en/concerts/",
		FirstSeen:   "2026-07-20",
	}
}

func TestValidBaseline(t *testing.T) {
	if p := Validate(File{Concerts: []Concert{valid()}}, nil, now); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

func TestFieldChecks(t *testing.T) {
	tests := []struct {
		name  string
		mutel func(*Concert)
	}{
		{"missing country", func(c *Concert) { c.Country = "" }},
		{"bad date format", func(c *Concert) { c.Date = "2026-9-12" }},
		{"impossible date", func(c *Concert) { c.Date = "2026-02-30"; c.ID = "scheps|2026-02-30|altenkrempe" }},
		{"year typo", func(c *Concert) { c.Date = "2072-09-12"; c.ID = "scheps|2072-09-12|altenkrempe" }},
		{"unknown tag", func(c *Concert) { c.LocationTag = "mars" }},
		{"javascript url", func(c *Concert) { c.SourceURL = "javascript:alert(1)" }},
		{"offlist domain", func(c *Concert) { c.SourceURL = "https://evil.example.com/x" }},
		{"id date mismatch", func(c *Concert) { c.ID = "scheps|2026-09-13|altenkrempe" }},
		{"id city mismatch", func(c *Concert) { c.ID = "scheps|2026-09-12|berlin" }},
		{"id bad shape", func(c *Concert) { c.ID = "scheps-altenkrempe" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.mutel(&c)
			if p := Validate(File{Concerts: []Concert{c}}, nil, now); len(p) == 0 {
				t.Fatalf("expected a problem for %q, got none", tt.name)
			}
		})
	}
}

func TestDuplicateID(t *testing.T) {
	c := valid()
	if p := Validate(File{Concerts: []Concert{c, c}}, nil, now); len(p) == 0 {
		t.Fatal("expected duplicate-id problem, got none")
	}
}

func TestAppendOnly(t *testing.T) {
	base := File{Concerts: []Concert{valid()}}

	// Adding a new entry is allowed.
	added := valid()
	added.ID = "scheps|2026-09-14|venice"
	added.Date = "2026-09-14"
	added.City = "Venice"
	added.SourceURL = "https://www.olgascheps.com/en/concerts/"
	if p := Validate(File{Concerts: []Concert{valid(), added}}, &base, now); len(p) != 0 {
		t.Fatalf("adding an entry should be allowed, got %v", p)
	}

	// Modifying an existing entry is rejected.
	modified := valid()
	modified.Venue = strptr("Somewhere Else")
	if p := Validate(File{Concerts: []Concert{modified}}, &base, now); len(p) == 0 {
		t.Fatal("modifying an existing entry should be rejected")
	}

	// Deleting an existing entry is rejected.
	if p := Validate(File{Concerts: []Concert{}}, &base, now); len(p) == 0 {
		t.Fatal("deleting an existing entry should be rejected")
	}
}

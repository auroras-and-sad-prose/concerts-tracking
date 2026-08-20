package main

import (
	"encoding/json"
	"testing"
	"time"
)

// now is fixed so the year-window check is deterministic in tests.
var now = time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

func strptr(s string) *string { return &s }

func pieceList(items ...string) Pieces { return Pieces{List: items} }
func pieceText(s string) Pieces        { return Pieces{Text: &s} }

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
		Pieces:      pieceList("Mozart String Quartet in F major KV 590"),
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
		{"missing pieces", func(c *Concert) { c.Pieces = Pieces{} }},
		{"empty pieces array", func(c *Concert) { c.Pieces = Pieces{List: []string{}} }},
		{"blank piece title", func(c *Concert) { c.Pieces = pieceList("Chopin Ballade No. 1", "  ") }},
		{"blank pieces string", func(c *Concert) { c.Pieces = pieceText("   ") }},
		{"empty detail_url", func(c *Concert) { c.DetailURL = strptr("  ") }},
		{"relative detail_url", func(c *Concert) { c.DetailURL = strptr("/programm/2026-09-12") }},
		{"javascript detail_url", func(c *Concert) { c.DetailURL = strptr("javascript:alert(1)") }},
		{"status without note", func(c *Concert) { c.Status = strptr("cancelled") }},
		{"status with blank note", func(c *Concert) { c.Status = strptr("cancelled"); c.StatusNote = strptr(" ") }},
		{"note without status", func(c *Concert) { c.StatusNote = strptr("called off") }},
		{"unknown status", func(c *Concert) { c.Status = strptr("rained off"); c.StatusNote = strptr("wet") }},
		{"empty instruments array", func(c *Concert) { c.Instruments = []string{} }},
		{"unknown instrument", func(c *Concert) { c.Instruments = []string{"theremin"} }},
		{"repeated instrument", func(c *Concert) { c.Instruments = []string{"piano", "piano"} }},
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

// The whole point of the field being optional: most rows won't state an
// instrument, and one that does states a real one.
func TestInstrumentsAcceptedShapes(t *testing.T) {
	for _, tt := range []struct {
		name string
		list []string
	}{
		{"absent", nil},
		{"single", []string{"piano"}},
		{"both", []string{"violin", "piano"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			c.Instruments = tt.list
			if p := Validate(File{Concerts: []Concert{c}}, nil, now); len(p) != 0 {
				t.Fatalf("expected no problems, got %v", p)
			}
		})
	}
}

// detail_url points at whatever page the listing links to for that concert —
// usually a promoter or venue site, which is exactly why it is not held to the
// source_url allowlist.
func TestDetailURLAcceptedShapes(t *testing.T) {
	for _, tt := range []struct {
		name string
		url  *string
	}{
		{"absent", nil},
		{"offlist promoter page", strptr("https://kempen-klassik.de/programm-details/olga-scheps-klavier-20260916.html")},
		{"bachtrack event page", strptr("https://bachtrack.com/concert-event/example/443111")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			c.DetailURL = tt.url
			if p := Validate(File{Concerts: []Concert{c}}, nil, now); len(p) != 0 {
				t.Fatalf("expected no problems, got %v", p)
			}
		})
	}
}

// A row can't be deleted or re-dated, so status is the only way the dataset
// can say a concert is off — and it only counts with the source's words behind
// it.
func TestStatusAcceptedShapes(t *testing.T) {
	for _, tt := range []struct {
		name         string
		status, note *string
	}{
		{"on as announced", nil, nil},
		{"cancelled", strptr("cancelled"), strptr("Tivoli: cancelled due to illness")},
		{"artist replaced", strptr("artist_replaced"), strptr("Karen Gomyo replaces Janine Jansen")},
		{"postponed", strptr("postponed"), strptr("moved to a date to be announced")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			c.Status, c.StatusNote = tt.status, tt.note
			if p := Validate(File{Concerts: []Concert{c}}, nil, now); len(p) != 0 {
				t.Fatalf("expected no problems, got %v", p)
			}
		})
	}
}

// A replacement that turns into an outright cancellation is a correction, not
// an erasure — but dropping the status entirely would quietly put a called-off
// concert back on the page.
func TestStatusMayBeCorrectedButNotDropped(t *testing.T) {
	off := valid()
	off.Status = strptr("artist_replaced")
	off.StatusNote = strptr("Karen Gomyo replaces Janine Jansen")
	base := File{Concerts: []Concert{off}}

	worse := off
	worse.Status = strptr("cancelled")
	worse.StatusNote = strptr("the concert is cancelled")
	if p := Validate(File{Concerts: []Concert{worse}}, &base, now); len(p) != 0 {
		t.Fatalf("correcting a status should be allowed, got %v", p)
	}

	back := off
	back.Status, back.StatusNote = nil, nil
	if p := Validate(File{Concerts: []Concert{back}}, &base, now); len(p) == 0 {
		t.Fatal("clearing a status on an existing entry should be rejected")
	}
}

func TestDuplicateID(t *testing.T) {
	c := valid()
	if p := Validate(File{Concerts: []Concert{c, c}}, nil, now); len(p) == 0 {
		t.Fatal("expected duplicate-id problem, got none")
	}
}

func TestPiecesAcceptedShapes(t *testing.T) {
	c := valid()
	c.Pieces = pieceText("Programme not announced")
	if p := Validate(File{Concerts: []Concert{c}}, nil, now); len(p) != 0 {
		t.Fatalf("a descriptive pieces string should be accepted, got %v", p)
	}
}

// Pieces is a hand-written union codec, so check both shapes survive a
// decode/encode round trip and that junk shapes are refused outright.
func TestPiecesRoundTrip(t *testing.T) {
	for _, in := range []string{
		`["Chopin Ballade No. 1","Chopin Ballade No. 2"]`,
		`"Composers only: Brahms, Schubert"`,
		`null`,
	} {
		var p Pieces
		if err := json.Unmarshal([]byte(in), &p); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		out, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal %s: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("round trip of %s produced %s", in, out)
		}
	}
}

func TestPiecesRejectsOtherShapes(t *testing.T) {
	for _, in := range []string{`42`, `true`, `{"work":"x"}`, `[1,2]`, `["ok",null]`} {
		var p Pieces
		if err := json.Unmarshal([]byte(in), &p); err == nil {
			t.Errorf("expected %s to be rejected, got %+v", in, p)
		}
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

	// Deleting an existing entry is rejected.
	if p := Validate(File{Concerts: []Concert{}}, &base, now); len(p) == 0 {
		t.Fatal("deleting an existing entry should be rejected")
	}

	// Identity fields are frozen once written.
	for _, tt := range []struct {
		name  string
		mutel func(*Concert)
	}{
		{"artist", func(c *Concert) { c.Artist = "Someone Else" }},
		{"first_seen", func(c *Concert) { c.FirstSeen = "2026-07-19" }},
	} {
		t.Run("immutable "+tt.name, func(t *testing.T) {
			c := valid()
			tt.mutel(&c)
			if p := Validate(File{Concerts: []Concert{c}}, &base, now); len(p) == 0 {
				t.Fatalf("changing %s on an existing entry should be rejected", tt.name)
			}
		})
	}
}

// The point of allowing updates: an event's details firm up over time, so a
// later run must be able to fill in a venue or turn a "composers only" note
// into a real list of works.
func TestExistingEntriesMayBeRefined(t *testing.T) {
	stub := valid()
	stub.Venue = nil
	stub.Pieces = pieceText("Composers only: Beethoven")
	base := File{Concerts: []Concert{stub}}

	refined := stub
	refined.Venue = strptr("Kurhaus Baden-Baden")
	refined.Program = strptr("Baden-Baden Philharmonic: Beethoven Piano Concerto No. 4")
	refined.Pieces = pieceList("Beethoven Piano Concerto No. 4")
	refined.Instruments = []string{"piano"}
	refined.LocationTag = "europe"
	refined.SourceURL = "https://bachtrack.com/performer/olga-scheps"
	refined.DetailURL = strptr("https://www.festspielhaus.de/en/events/beethoven-4/")

	if p := Validate(File{Concerts: []Concert{refined}}, &base, now); len(p) != 0 {
		t.Fatalf("refining descriptive fields should be allowed, got %v", p)
	}
}

func TestExistingDetailMayNotBeErased(t *testing.T) {
	withInstrument := valid()
	withInstrument.Instruments = []string{"piano"}
	withInstrument.DetailURL = strptr("https://kempen-klassik.de/programm-details/olga-scheps-klavier-20260916.html")
	base := File{Concerts: []Concert{withInstrument}}

	for _, tt := range []struct {
		name  string
		mutel func(*Concert)
	}{
		{"venue", func(c *Concert) { c.Venue = nil }},
		{"pieces", func(c *Concert) { c.Pieces = Pieces{} }},
		{"instruments", func(c *Concert) { c.Instruments = nil }},
		{"detail_url", func(c *Concert) { c.DetailURL = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Start from the fully populated row so each case erases exactly
			// one field and the others stay intact.
			c := withInstrument
			tt.mutel(&c)
			if p := Validate(File{Concerts: []Concert{c}}, &base, now); len(p) == 0 {
				t.Fatalf("clearing %s on an existing entry should be rejected", tt.name)
			}
		})
	}
}

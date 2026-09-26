package stations

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture mirrors real rows of de.csv for the cases below.
const fixture = `name;eva
Bad Wörishofen;8000768
Bamberg;8000025
Bamberg Bahnhof;8089478
Coesfeld (Westf);8000066
Coesfeld Schulzentrum;8001343
Frankfurt (Main) Hbf;8000105
Frankfurt (Main) Süd;8002041
Frankfurt (Oder);8010113
Kempen (Niederrhein);8000409
Künzelsau Bf;8087022
München Hbf;8000261
München Hbf Gleis 27-36;8098261
Neustadt (Holst);8004349
Neustadt (Weinstr) Hbf;8000275
Passau Hbf;8000298
Passau Hbf;8070567
`

func load(t *testing.T) *List {
	t.Helper()
	path := filepath.Join(t.TempDir(), "de.csv")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestFindPicksOne(t *testing.T) {
	l := load(t)
	for _, tt := range []struct{ query, name, eva string }{
		{"München", "München Hbf", "8000261"},           // main station beats the platform-group entries
		{"Bamberg", "Bamberg", "8000025"},               // exact name beats "Bamberg Bahnhof"
		{"Bad Wörishofen", "Bad Wörishofen", "8000768"}, // the ID bahn.de itself used
		{"Künzelsau", "Künzelsau Bf", "8087022"},        // "<name> Bf"
		{"Kempen", "Kempen (Niederrhein)", "8000409"},   // the only qualified Kempen
		{"Coesfeld", "Coesfeld (Westf)", "8000066"},     // a qualifier, not the school stop
		{"Frankfurt (Main)", "Frankfurt (Main) Hbf", "8000105"},
		{"  Bamberg ", "Bamberg", "8000025"},
	} {
		r := l.Find(tt.query)
		if r.Station == nil || r.Station.Name != tt.name || r.Station.EVA != tt.eva {
			t.Errorf("Find(%q) = %+v, want %s;%s", tt.query, r, tt.name, tt.eva)
		}
	}
}

// A name that fits several stations equally well is returned as a tie, never
// resolved by picking one.
func TestFindReportsTies(t *testing.T) {
	l := load(t)
	for _, q := range []string{"Frankfurt", "Neustadt", "Passau"} {
		r := l.Find(q)
		if r.Station != nil || len(r.Candidates) < 2 {
			t.Errorf("Find(%q) = %+v, want a tie", q, r)
		}
	}
}

func TestFindNothing(t *testing.T) {
	l := load(t)
	for _, q := range []string{"Altenkrempe", "", "Munich"} {
		if r := l.Find(q); r.Station != nil || len(r.Candidates) != 0 {
			t.Errorf("Find(%q) = %+v, want nothing", q, r)
		}
	}
}

func TestHas(t *testing.T) {
	l := load(t)
	if !l.Has(Station{"Kempen (Niederrhein)", "8000409"}) {
		t.Error("expected the Kempen row to be found")
	}
	for _, s := range []Station{{"Kempen (Niederrhein)", "8000410"}, {"Kempen", "8000409"}} {
		if l.Has(s) {
			t.Errorf("Has(%+v) = true, want false", s)
		}
	}
}

func TestLoadRejectsMalformedFiles(t *testing.T) {
	for name, content := range map[string]string{
		"wrong header": "station;id\nBamberg;8000025\n",
		"no number":    "name;eva\nBamberg\n",
		"bad number":   "name;eva\nBamberg;80000x5\n",
		"empty name":   "name;eva\n;8000025\n",
		"no rows":      "name;eva\n",
	} {
		path := filepath.Join(t.TempDir(), "de.csv")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// The checked-in file must load: CI validates travel.json against it.
func TestCheckedInFileLoads(t *testing.T) {
	l, err := Load("de.csv")
	if err != nil {
		t.Fatal(err)
	}
	if r := l.Find("Bad Wörishofen"); r.Station == nil || r.Station.EVA != "8000768" {
		t.Errorf("Find(Bad Wörishofen) on de.csv = %+v", r)
	}
}

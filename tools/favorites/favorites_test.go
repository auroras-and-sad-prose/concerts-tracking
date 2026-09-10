package favorites

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// cases mirrors testdata/cases.json, the fixture this package shares with the
// browser smoke tests so that the Go matcher and the JavaScript mirror in
// index.html are held to the same answers.
type cases struct {
	Normalize []struct {
		In  string `json:"in"`
		Out string `json:"out"`
	} `json:"normalize"`
	Favorites []Favorite `json:"favorites"`
	Match     []struct {
		Piece string   `json:"piece"`
		Slugs []string `json:"slugs"`
	} `json:"match"`
}

func loadCases(t *testing.T) cases {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "cases.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var c cases
	// Not DisallowUnknownFields: the fixture carries a _comment block for the
	// humans maintaining it.
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	if len(c.Normalize) == 0 || len(c.Favorites) == 0 || len(c.Match) == 0 {
		t.Fatal("fixture is missing one of its three sections")
	}
	return c
}

func TestNormalizeSharedCases(t *testing.T) {
	for _, tc := range loadCases(t).Normalize {
		if got := Normalize(tc.In); got != tc.Out {
			t.Errorf("Normalize(%q) = %q, want %q", tc.In, got, tc.Out)
		}
	}
}

func TestMatchSharedCases(t *testing.T) {
	c := loadCases(t)
	f := File{Favorites: c.Favorites}
	if problems := Validate(f); len(problems) > 0 {
		t.Fatalf("fixture favorites do not validate: %v", problems)
	}
	for _, tc := range c.Match {
		got := Slugs(f.Hits([]string{tc.Piece}))
		want := tc.Slugs
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("Hits(%q) = %v, want %v", tc.Piece, got, want)
		}
	}
}

// The fold table is two parallel strings, which is only correct while they stay
// the same length; buildFoldSingle panics otherwise, and this says so out loud
// rather than as an obscure init failure.
func TestFoldTableIsBalanced(t *testing.T) {
	if from, to := []rune(foldFrom), []rune(foldTo); len(from) != len(to) {
		t.Fatalf("fold table has %d source runes but %d targets", len(from), len(to))
	}
}

func TestNormalizeFoldsWithoutJoiningTokens(t *testing.T) {
	// A hyphen, a slash and an em-dash are all boundaries: folding them away
	// entirely would let "g-Moll" be matched as the single token "gmoll", and
	// a term nobody wrote would start matching.
	for _, in := range []string{"g-Moll", "g/Moll", "g — Moll", "g.Moll"} {
		if got := Normalize(in); got != "g moll" {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, "g moll")
		}
	}
}

func TestHitsReportsEveryPieceAndFavorite(t *testing.T) {
	f := File{Favorites: []Favorite{
		{Slug: "chopin-ballade-1", Title: "Chopin Ballade No. 1", Patterns: [][]string{{"chopin", "ballade", "no 1"}}},
		{Slug: "chopin-any", Title: "Anything by Chopin", Patterns: [][]string{{"chopin"}}},
	}}

	hits := f.Hits([]string{"Mozart Sonata KV 310", "Chopin Ballade No. 1", "Chopin Fantasy in F minor"})
	want := []Hit{
		{PieceIndex: 1, Slug: "chopin-ballade-1", Title: "Chopin Ballade No. 1"},
		{PieceIndex: 1, Slug: "chopin-any", Title: "Anything by Chopin"},
		{PieceIndex: 2, Slug: "chopin-any", Title: "Anything by Chopin"},
	}
	if !slices.Equal(hits, want) {
		t.Errorf("Hits = %v, want %v", hits, want)
	}

	// A favorite found twice is still one favorite when the row is reported.
	if got := Slugs(hits); !slices.Equal(got, []string{"chopin-ballade-1", "chopin-any"}) {
		t.Errorf("Slugs = %v", got)
	}
	if got := Titles(hits); !slices.Equal(got, []string{"Chopin Ballade No. 1", "Anything by Chopin"}) {
		t.Errorf("Titles = %v", got)
	}
}

func TestTermsMatchOnTokenBoundaries(t *testing.T) {
	f := File{Favorites: []Favorite{
		{Slug: "op-2", Title: "Op. 2", Patterns: [][]string{{"op 2"}}},
	}}
	if hits := f.Hits([]string{"Brahms Piano Sonata in C major, Op. 23"}); len(hits) != 0 {
		t.Errorf("op 2 matched inside op 23: %v", hits)
	}
	if hits := f.Hits([]string{"Brahms Piano Sonata in C major, Op. 2"}); len(hits) != 1 {
		t.Errorf("op 2 did not match Op. 2: %v", hits)
	}
}

func TestValidateAcceptsAWellFormedFile(t *testing.T) {
	f := File{Favorites: []Favorite{
		{Slug: "chopin-ballade-1", Title: "Chopin Ballade No. 1", Patterns: [][]string{{"chopin", "ballade", "no 1"}}},
	}}
	if problems := Validate(f); len(problems) > 0 {
		t.Errorf("expected no problems, got %v", problems)
	}
}

// An empty list is a legitimate state — nothing starred yet, or everything
// un-starred again — and must not be an error, or clearing the file would
// require a code change.
func TestValidateAcceptsAnEmptyList(t *testing.T) {
	if problems := Validate(File{Favorites: []Favorite{}}); len(problems) > 0 {
		t.Errorf("expected no problems for an empty list, got %v", problems)
	}
}

func TestValidateRejects(t *testing.T) {
	good := Favorite{Slug: "chopin-ballade-1", Title: "Chopin Ballade No. 1", Patterns: [][]string{{"chopin", "ballade"}}}

	tests := []struct {
		name string
		file File
		want string
	}{
		{
			name: "slug with spaces",
			file: File{Favorites: []Favorite{{Slug: "chopin ballade", Title: "T", Patterns: [][]string{{"chopin"}}}}},
			want: "slug",
		},
		{
			name: "duplicate slug",
			file: File{Favorites: []Favorite{good, {Slug: good.Slug, Title: "Other", Patterns: [][]string{{"x"}}}}},
			want: "duplicate slug",
		},
		{
			name: "duplicate title",
			file: File{Favorites: []Favorite{good, {Slug: "other", Title: good.Title, Patterns: [][]string{{"x"}}}}},
			want: "duplicate title",
		},
		{
			name: "missing title",
			file: File{Favorites: []Favorite{{Slug: "x", Title: "  ", Patterns: [][]string{{"x"}}}}},
			want: `field "title" is required`,
		},
		{
			name: "no patterns",
			file: File{Favorites: []Favorite{{Slug: "x", Title: "T"}}},
			want: `field "patterns" is required`,
		},
		{
			name: "empty pattern would match everything",
			file: File{Favorites: []Favorite{{Slug: "x", Title: "T", Patterns: [][]string{{}}}}},
			want: "would match every work",
		},
		{
			name: "term with nothing to match on",
			file: File{Favorites: []Favorite{{Slug: "x", Title: "T", Patterns: [][]string{{"chopin", "—"}}}}},
			want: "no letters or digits",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := Validate(tc.file)
			if len(problems) == 0 {
				t.Fatalf("expected a problem mentioning %q, got none", tc.want)
			}
			if !containsSubstring(problems, tc.want) {
				t.Errorf("expected a problem mentioning %q, got %v", tc.want, problems)
			}
		})
	}
}

// A pattern that Validate rejects must also never match at run time: the file
// could reach the page before anyone fixes it, and a match-all favorite would
// star every concert in the dataset.
func TestEmptyPatternNeverMatches(t *testing.T) {
	f := File{Favorites: []Favorite{{Slug: "x", Title: "T", Patterns: [][]string{{}, {"nothing here"}}}}}
	if hits := f.Hits([]string{"Chopin Ballade No. 1"}); len(hits) != 0 {
		t.Errorf("empty pattern matched: %v", hits)
	}
}

func containsSubstring(problems []string, want string) bool {
	for _, p := range problems {
		if strings.Contains(p, want) {
			return true
		}
	}
	return false
}

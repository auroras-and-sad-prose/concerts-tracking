// Package favorites implements the curated favourite-works list and the rule
// that decides whether a concert's programme contains one of them.
//
// The list is hand-maintained in favorites.json, exactly like the artist
// roster: which works you care about is a fact about you, not about any
// concert, so the concert-watch routine never writes this file.
//
// # Why nothing is written into seen.json
//
// "This concert plays a favourite" is a function of two things already in the
// repo — the row's pieces and the curated list — so it is computed wherever it
// is needed (the page, the run report) rather than stored on the row. A
// favorites field on a concert would be one more field a run could get wrong,
// gating nothing that the two inputs don't already gate, and it would go stale
// the moment the curated list changed. Deriving it also means adding a
// favourite immediately lights up every concert already in the dataset.
//
// # How a work is matched
//
// Sources phrase the same work a dozen ways — "Chopin Ballade No. 1",
// "Ballade Nr. 1 g-Moll op. 23", "F. Chopin: Ballade No.1" — and the routine
// copies whatever the page printed rather than normalising it (grounding rule
// 3). So a favourite does not carry one canonical title to compare against; it
// carries the phrasings that identify it, curated by a person:
//
//	"patterns": [["chopin", "ballade", "no 1"],
//	             ["chopin", "ballade", "nr 1"],
//	             ["ballade", "op 23"]]
//
// A favourite matches a work title when every term of any one pattern appears
// in it (OR of ANDs). Both sides are normalised first — case, accents and
// punctuation folded away — and a term matches only on whole-token boundaries,
// so "op 2" does not match "Op. 23".
//
// Matching is deliberately literal. It knows nothing about the repertoire: it
// cannot decide that "Beethoven Op. 61" is the violin concerto, because that is
// knowledge from outside the fetched page, which is precisely what the rest of
// this repo exists to keep out of the data. A phrasing the patterns don't cover
// is a miss, and the fix is a person adding a pattern — never a run guessing.
//
// # Two implementations
//
// index.html carries a JavaScript mirror of Normalize and the matching rule, so
// that the page can highlight favourites without a build step. The two must
// agree; testdata/cases.json is the shared conformance fixture that proves they
// do, read by favorites_test.go here and by the browser smoke tests.
package favorites

import (
	"fmt"
	"regexp"
	"strings"
)

// slugRe is kebab-case, unlike the artist slugs in artists.json: a work needs
// composer and number to be identifiable ("chopin-ballade-1"), where an artist
// slug is one surname.
var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Favorite is one entry in favorites.json.
type Favorite struct {
	// Slug identifies the entry — in the page's filter, and in a run's report.
	Slug string `json:"slug"`
	// Title is what a human reads: the work as you'd name it, not as any
	// source phrases it. It is never matched against — Patterns does that.
	Title string `json:"title"`
	// Patterns are the alternative phrasings that identify this work. The
	// favourite matches a title when all terms of any one pattern appear in it.
	Patterns [][]string `json:"patterns"`
}

// File is the top-level shape of favorites.json.
type File struct {
	Favorites []Favorite `json:"favorites"`
}

// Hit records one favourite found in one element of a row's pieces array.
type Hit struct {
	PieceIndex int    // index into the pieces array that matched
	Slug       string // the favourite's slug
	Title      string // the favourite's curated title
}

// Validate checks the curated file in isolation: well-formed slugs and titles,
// and patterns that can actually match something. An empty list is allowed —
// "I have no favourites recorded yet" is a legitimate state, and the page just
// hides the control.
func Validate(f File) []string {
	var problems []string

	slugs := make(map[string]int, len(f.Favorites))
	titles := make(map[string]int, len(f.Favorites))
	for i, fav := range f.Favorites {
		label := fav.Slug
		if label == "" {
			label = fmt.Sprintf("favorites[%d]", i)
		}

		if !slugRe.MatchString(fav.Slug) {
			problems = append(problems, fmt.Sprintf(
				"%s: slug %q must be lowercase letters, digits and single hyphens", label, fav.Slug))
		} else if prev, dup := slugs[fav.Slug]; dup {
			problems = append(problems, fmt.Sprintf("%s: duplicate slug (also favorites[%d])", label, prev))
		} else {
			slugs[fav.Slug] = i
		}

		// Titles label the page's filter options, so two identical ones would
		// produce two indistinguishable entries in the same dropdown.
		if strings.TrimSpace(fav.Title) == "" {
			problems = append(problems, fmt.Sprintf("%s: field %q is required but empty", label, "title"))
		} else if prev, dup := titles[fav.Title]; dup {
			problems = append(problems, fmt.Sprintf(
				"%s: duplicate title %q (also favorites[%d])", label, fav.Title, prev))
		} else {
			titles[fav.Title] = i
		}

		if len(fav.Patterns) == 0 {
			problems = append(problems, fmt.Sprintf(
				"%s: field %q is required (at least one phrasing that identifies the work)", label, "patterns"))
		}
		for j, pattern := range fav.Patterns {
			if len(pattern) == 0 {
				problems = append(problems, fmt.Sprintf(
					"%s: patterns[%d] is empty; a pattern with no terms would match every work", label, j))
				continue
			}
			for k, term := range pattern {
				// A term that normalises away to nothing — punctuation, an
				// em-dash, whitespace — would be trivially "found" in every
				// title, quietly turning its pattern into a match-all.
				if Normalize(term) == "" {
					problems = append(problems, fmt.Sprintf(
						"%s: patterns[%d][%d] %q has no letters or digits to match on", label, j, k, term))
				}
			}
		}
	}

	return problems
}

// Hits reports every favourite found in a row's pieces, in pieces order and
// then in file order, one Hit per (piece, favourite) pair so the page can
// highlight the individual work that matched.
//
// It is only ever given the array form of pieces. The string form —
// "Programme not announced", "Composers only: Chopin" — says the works are
// unknown, and reading a favourite out of it would turn "we don't know" into
// "your piece is on the bill".
func (f File) Hits(pieces []string) []Hit {
	var hits []Hit
	for i, piece := range pieces {
		normalized := Normalize(piece)
		if normalized == "" {
			continue
		}
		for _, fav := range f.Favorites {
			if fav.matches(normalized) {
				hits = append(hits, Hit{PieceIndex: i, Slug: fav.Slug, Title: fav.Title})
			}
		}
	}
	return hits
}

// Titles reduces a row's hits to the distinct favourites found, in first-seen
// order — what a report or an alert wants to name.
func Titles(hits []Hit) []string {
	seen := make(map[string]bool, len(hits))
	var titles []string
	for _, h := range hits {
		if !seen[h.Slug] {
			seen[h.Slug] = true
			titles = append(titles, h.Title)
		}
	}
	return titles
}

// Slugs reduces a row's hits to the distinct favourites found, in first-seen
// order.
func Slugs(hits []Hit) []string {
	seen := make(map[string]bool, len(hits))
	var slugs []string
	for _, h := range hits {
		if !seen[h.Slug] {
			seen[h.Slug] = true
			slugs = append(slugs, h.Slug)
		}
	}
	return slugs
}

// matches reports whether any one of the favourite's patterns is wholly present
// in an already-normalised work title.
func (f Favorite) matches(normalizedTitle string) bool {
	for _, pattern := range f.Patterns {
		if len(pattern) == 0 {
			continue // rejected by Validate; never treat it as "matches everything"
		}
		all := true
		for _, term := range pattern {
			t := Normalize(term)
			if t == "" || !containsTerm(normalizedTitle, t) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// containsTerm looks for a term as a whole run of tokens rather than as a bare
// substring. Both arguments are normalised, so tokens are separated by exactly
// one space and padding both sides turns the substring test into a boundary
// test: " op 2 " is not found in " ... op 23 ", though "op 2" is a substring of
// "op 23".
func containsTerm(haystack, term string) bool {
	return strings.Contains(" "+haystack+" ", " "+term+" ")
}

// Fold pairs: one lowercase Latin letter carrying diacritics, and the plain
// letter it folds to. Covers Latin-1 Supplement and Latin Extended-A, which is
// every accented letter European composer and work names actually use.
//
// The pairs are applied directly rather than by Unicode decomposition because
// the JavaScript mirror in index.html applies this same table: two tables that
// are equal by construction cannot drift, where "Go decomposes, JS calls
// normalize('NFD')" would differ on every letter that has no decomposition —
// ø, ł, ß, æ — in ways nobody would notice until a favourite silently stopped
// matching.
const (
	foldFrom = "àáâãäåāăą" + "çćĉċč" + "ďđ" + "èéêëēĕėęě" + "ĝğġģ" + "ĥħ" +
		"ìíîïĩīĭįı" + "ĵ" + "ķ" + "ĺļľŀł" + "ñńņň" + "òóôõöōŏőø" + "ŕŗř" +
		"śŝşš" + "ţťŧ" + "ùúûüũūŭůűų" + "ŵ" + "ýÿŷ" + "źżž"
	foldTo = "aaaaaaaaa" + "ccccc" + "dd" + "eeeeeeeee" + "gggg" + "hh" +
		"iiiiiiiii" + "j" + "k" + "lllll" + "nnnn" + "ooooooooo" + "rrr" +
		"ssss" + "ttt" + "uuuuuuuuuu" + "w" + "yyy" + "zzz"
)

// foldExpand holds the letters that fold to more than one character, and so
// cannot live in the paired strings above.
var foldExpand = map[rune]string{
	'ß': "ss", 'æ': "ae", 'œ': "oe", 'ĳ': "ij", 'þ': "th", 'ð': "d",
}

var foldSingle = buildFoldSingle()

func buildFoldSingle() map[rune]rune {
	from, to := []rune(foldFrom), []rune(foldTo)
	if len(from) != len(to) {
		// A programming error in the constants above, not a data problem:
		// fail loudly at startup rather than fold half the alphabet wrongly.
		panic(fmt.Sprintf("favorites: fold table mismatch: %d source runes, %d targets", len(from), len(to)))
	}
	m := make(map[rune]rune, len(from))
	for i, r := range from {
		m[r] = to[i]
	}
	return m
}

// Normalize reduces a work title to the form both the curated patterns and the
// sources' phrasings are compared in: lowercase, unaccented, and stripped of
// punctuation down to space-separated tokens.
//
//	`Beethoven: Piano Sonata No. 21 in C major "Waldstein", Op.53`
//	→ "beethoven piano sonata no 21 in c major waldstein op 53"
//	`Max Bruch: Violinkonzert Nr. 1 g-Moll op. 26`
//	→ "max bruch violinkonzert nr 1 g moll op 26"
//
// It folds accents but does not translate: "Violinkonzert" and "Violin
// Concerto" stay different words, which is why a favourite carries a pattern
// for each language a source might print it in.
//
// The JavaScript mirror in index.html must produce byte-identical output;
// testdata/cases.json is what holds them to it.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := true // leading spaces are never emitted
	emit := func(r rune) {
		b.WriteRune(r)
		space = false
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 0x0300 && r <= 0x036F:
			// A combining mark: drop it, so a title that arrives already
			// decomposed ("a" + U+0301) folds the same way a precomposed "á"
			// does. This also disposes of the combining dot that lowercasing
			// Turkish "İ" leaves behind.
			continue
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			emit(r)
		default:
			if folded, ok := foldSingle[r]; ok {
				emit(folded)
				continue
			}
			if expanded, ok := foldExpand[r]; ok {
				b.WriteString(expanded)
				space = false
				continue
			}
			// Everything else — punctuation, symbols, scripts we don't fold —
			// becomes a token boundary rather than being dropped, so "g-Moll"
			// reads as two tokens and cannot be matched as "gmoll".
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimRight(b.String(), " ")
}

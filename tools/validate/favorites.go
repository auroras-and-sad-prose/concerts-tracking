// favorites.json is the curated list of works worth travelling for. Like
// artists.json it is hand-maintained and never written by the concert-watch
// routine — see tools/favorites for the file's shape and the matching rule.
//
// Nothing in seen.json points at it, so there is no cross-check to run the way
// CheckRoster ties concerts to artists: a favorite that no concert plays is
// the ordinary case, and a concert playing none of them is too. What this file
// adds instead is the report — which upcoming concerts play a favorite, and
// which of those the reader has not been told about yet.
//
// The report exists so that a run alerts by copying a computed answer rather
// than by reading programmes and deciding for itself which works count. That
// judgement is a person's, made once when they curate a pattern, not remade
// from memory on every run. So each line it prints is a finished ★ Favorites
// entry in the shape CLAUDE.md's step 7 asks for — artist, date, place, the
// curated title, the words the source actually printed, and the row's links —
// grouped under the section of the issue it belongs to. Nothing is left for
// the run to look up and reassemble, because reassembly is where a URL gets
// attached to the wrong concert.
package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/favorites"
)

// bucket is which part of the run's alert a matched favorite belongs to.
type bucket int

const (
	// bucketFlagged outranks the rest: a concert that is off, moved or handed
	// to another soloist is not good news whatever it was going to play.
	bucketFlagged  bucket = iota
	bucketNew             // the concert itself is new to the dataset
	bucketNewMatch        // known concert whose programme now names a favorite
	bucketKnown           // already reported in an earlier run
)

// favoriteLine is one (concert, favorite) pair — the unit the issue lists,
// since a concert playing two favorites earns a line for each.
type favoriteLine struct {
	concert Concert
	hit     favorites.Hit
	bucket  bucket
}

// reportFavorites writes the favorites report to w. It never affects the exit
// code: the curated list is a lens on valid data, not another gate on it.
//
// With base non-nil it also says what changed, which is what a run alerts on: a
// concert new to the file, or an existing row whose programme has since been
// filled in and turns out to contain a favorite. Both are computed against the
// current list, so this reports news about concerts, not about the list — after
// starring a new work, run it without -base to see every concert it catches.
func reportFavorites(w io.Writer, f File, base *File, fav favorites.File, now time.Time) {
	if len(fav.Favorites) == 0 {
		fmt.Fprintln(w, "favorites: the list is empty; nothing to match")
		return
	}

	today := now.Format(dateLayout)

	// What the base file already matched, so that "new" means new to the
	// reader rather than new to this comparison.
	baseSlugs := map[string]map[string]bool{}
	if base != nil {
		for _, c := range base.Concerts {
			slugs := map[string]bool{}
			for _, s := range favorites.Slugs(fav.Hits(c.Pieces.List)) {
				slugs[s] = true
			}
			baseSlugs[c.ID] = slugs
		}
	}

	var lines []favoriteLine
	upcoming, concerts := 0, 0
	for _, c := range f.Concerts {
		if c.Date < today {
			continue
		}
		upcoming++
		hits := fav.Hits(c.Pieces.List)
		if len(hits) == 0 {
			continue
		}
		concerts++
		known, existed := baseSlugs[c.ID]
		for _, h := range hits {
			b := bucketKnown
			switch {
			case c.Status != nil:
				b = bucketFlagged
			case base == nil:
				// Nothing to compare against: every match is simply current.
			case !existed:
				b = bucketNew
			case !known[h.Slug]:
				b = bucketNewMatch
			}
			lines = append(lines, favoriteLine{concert: c, hit: h, bucket: b})
		}
	}

	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].concert.Date != lines[j].concert.Date {
			return lines[i].concert.Date < lines[j].concert.Date
		}
		return lines[i].concert.ID < lines[j].concert.ID
	})

	if len(lines) == 0 {
		fmt.Fprintf(w, "favorites: none of the %d upcoming concert(s) plays a work on the list (%d favorite(s))\n",
			upcoming, len(fav.Favorites))
		return
	}

	// Both counts, because they differ whenever one bill plays two starred
	// works: the sections below count entries, one per work, since that is
	// what the issue lists.
	fmt.Fprintf(w, "favorites: %d of %d upcoming concert(s) play a work on the list (%d entries)\n",
		concerts, upcoming, len(lines))

	section(w, lines, bucketNew,
		"new concerts playing a favorite — copy into the issue's ★ Favorites section", "")
	section(w, lines, bucketNewMatch,
		"programmes that now name a favorite — ★ Favorites as well", " (programme now announced)")
	if base == nil {
		section(w, lines, bucketKnown, "upcoming concerts playing a favorite", "")
	} else if n := count(lines, bucketKnown); n > 0 {
		// Listed only as a count when there is a base to compare against:
		// these were reported when they were found, and printing them beside
		// this run's news is how they get reported a second time.
		fmt.Fprintf(w, "\n  already reported in an earlier run, not news now: %d\n", n)
	}
	section(w, lines, bucketFlagged,
		"playing a favorite but flagged — report under Changes, never under ★ Favorites", "")
}

// section prints one group of lines under its heading, or nothing when the
// group is empty. suffix is appended to every line in the group.
func section(w io.Writer, lines []favoriteLine, b bucket, heading, suffix string) {
	n := count(lines, b)
	if n == 0 {
		return
	}
	fmt.Fprintf(w, "\n  %s (%d):\n", heading, n)
	for _, l := range lines {
		if l.bucket == b {
			fmt.Fprintf(w, "    %s%s\n", issueLine(l), suffix)
		}
	}
}

func count(lines []favoriteLine, b bucket) int {
	n := 0
	for _, l := range lines {
		if l.bucket == b {
			n++
		}
	}
	return n
}

// issueLine renders one matched favorite in the shape the issue wants:
//
//	artist — date — city, venue — title (matched: "…") — source_url — detail_url
//
// The matched phrasing travels with the title for the same reason status_note
// travels with status: the claim is only as good as the words the page printed,
// and a reader who disagrees with a match can see what triggered it without
// opening the row. A flagged concert carries its status instead of pretending
// to be an invitation.
func issueLine(l favoriteLine) string {
	c := l.concert
	where := c.City
	if c.Venue != nil && strings.TrimSpace(*c.Venue) != "" {
		where += ", " + *c.Venue
	}

	parts := []string{
		c.Artist,
		c.Date,
		where,
		fmt.Sprintf("%s (matched: %q)", l.hit.Title, c.Pieces.List[l.hit.PieceIndex]),
		c.SourceURL,
	}
	if c.DetailURL != nil && strings.TrimSpace(*c.DetailURL) != "" {
		parts = append(parts, *c.DetailURL)
	}
	if c.Status != nil {
		note := ""
		if c.StatusNote != nil {
			note = ": " + strings.TrimSpace(*c.StatusNote)
		}
		parts = append(parts, fmt.Sprintf("[%s%s]", *c.Status, note))
	}
	return strings.Join(parts, " — ")
}

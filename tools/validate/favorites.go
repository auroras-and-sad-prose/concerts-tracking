// favorites.json is the curated list of works worth travelling for. Like
// artists.json it is hand-maintained and never written by the concert-watch
// routine — see tools/favorites for the file's shape and the matching rule.
//
// Nothing in seen.json points at it, so there is no cross-check to run the way
// CheckRoster ties concerts to artists: a favourite that no concert plays is
// the ordinary case, and a concert playing none of them is too. What this file
// adds instead is the report — which upcoming concerts play a favourite, and
// which of those the reader has not been told about yet.
//
// The report exists so that a run alerts by copying a computed answer rather
// than by reading programmes and deciding for itself which works count. That
// judgement is a person's, made once when they curate a pattern, not remade
// from memory on every run.
package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/favorites"
)

// favoriteRow is one upcoming concert that plays at least one favourite.
type favoriteRow struct {
	concert Concert
	hits    []favorites.Hit
	isNew   bool     // the row itself is not in the base file
	newly   []string // slugs matched now that were not matched in the base file
}

// reportFavorites writes the favourites report to w. It never affects the exit
// code: the curated list is a lens on valid data, not another gate on it.
//
// With base non-nil it also says what changed, which is what a run alerts on: a
// concert new to the file, or an existing row whose programme has since been
// filled in and turns out to contain a favourite. Both are computed against the
// current list, so this reports news about concerts, not about the list — after
// starring a new work, run it without -base to see every concert it catches.
func reportFavorites(w io.Writer, f File, base *File, fav favorites.File, now time.Time) {
	if len(fav.Favorites) == 0 {
		fmt.Fprintln(w, "favourites: the list is empty; nothing to match")
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

	var rows []favoriteRow
	upcoming := 0
	for _, c := range f.Concerts {
		if c.Date < today {
			continue
		}
		upcoming++
		hits := fav.Hits(c.Pieces.List)
		if len(hits) == 0 {
			continue
		}
		row := favoriteRow{concert: c, hits: hits}
		if base != nil {
			known, existed := baseSlugs[c.ID]
			row.isNew = !existed
			for _, s := range favorites.Slugs(hits) {
				if existed && !known[s] {
					row.newly = append(row.newly, s)
				}
			}
		}
		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].concert.Date != rows[j].concert.Date {
			return rows[i].concert.Date < rows[j].concert.Date
		}
		return rows[i].concert.ID < rows[j].concert.ID
	})

	if len(rows) == 0 {
		fmt.Fprintf(w, "favourites: none of the %d upcoming concert(s) plays a work on the list (%d favourite(s))\n",
			upcoming, len(fav.Favorites))
		return
	}

	fmt.Fprintf(w, "favourites: %d of %d upcoming concert(s) play a work on the list\n", len(rows), upcoming)
	for _, row := range rows {
		fmt.Fprintf(w, "  %s\n", favoriteHeadline(row))
		newly := map[string]bool{}
		for _, s := range row.newly {
			newly[s] = true
		}
		for _, h := range row.hits {
			mark := ""
			if newly[h.Slug] && !row.isNew {
				mark = "   NEW MATCH"
			}
			fmt.Fprintf(w, "      %s   [from %q]%s\n", h.Title, row.concert.Pieces.List[h.PieceIndex], mark)
		}
	}
}

// favoriteHeadline is the concert's one-line billing. A status is printed loudly
// beside it: a cancelled concert can still match a favourite, and reporting it
// as good news is exactly the mistake status exists to prevent.
func favoriteHeadline(row favoriteRow) string {
	c := row.concert
	where := c.City
	if c.Venue != nil && strings.TrimSpace(*c.Venue) != "" {
		where += ", " + *c.Venue
	}
	line := fmt.Sprintf("%s  %s — %s", c.Date, c.Artist, where)
	if c.Status != nil {
		line += fmt.Sprintf("  [%s]", *c.Status)
	}
	if row.isNew {
		line += "  NEW CONCERT"
	}
	return line
}

# concerts-tracking

`seen.json` is a curated dataset of upcoming classical concerts, populated by an
automated concert-watch routine and rendered by `index.html` (GitHub Pages).
`artists.json` is the hand-maintained roster of the musicians tracked and the
instrument(s) each one plays — see "The artist roster" below.

Reducing hallucination in this dataset relies on three layers: the **enforced
layer**, which is what actually gates the data (CI); the **operating
procedure**, which is the routine's run script; and the **grounding rules**,
the principles that procedure leans on to avoid producing bad rows in the first
place. This file is the routine's full instructions — it isn't handed a
separate prompt.

## Enforced layer (CI — cannot be hallucinated past)

`tools/validate` runs in CI (`.github/workflows/validate.yml`) on every change to
`seen.json`. It fails the build when an entry:

- is missing a required field, or has an unknown/extra field;
- has a `date` or `first_seen` that is not a real zero-padded `YYYY-MM-DD`;
- has a concert year outside `now-1 .. now+3` (catches typoed years);
- has a `location_tag` outside the allowed set (`europe`, `germany`, `berlin`);
- has a `source_url` that is not http(s) on an allowlisted domain
  (`ilyunburkev.com`, `olgascheps.com`, `mariaduenasviolin.com`,
  `mayaoganyan.com`, `janinejansen.com`, `juliafischer.com`,
  `itzhakperlman.com`, `bachtrack.com`);
- has a `pieces` that is absent, an empty array, an empty string, or any shape
  other than an array of non-empty strings or a single non-empty string;
- has an `id` whose shape isn't `<slug>|<date>|<city>` or whose date/city
  segments disagree with the row's own fields;
- duplicates another entry's `id`;
- names an artist absent from `artists.json`, or whose `artist` string disagrees
  with the name registered there for that id slug.

It also constrains how existing entries may change. Entries may never be
**deleted**, and the fields that pin a row to one specific concert — `artist`,
`date`, `city` — plus its provenance, `first_seen`, are **frozen** once written:
rewriting one of those would quietly repoint a vetted row at a different event
while keeping its id. (`id` itself is the identity key, and the id-shape rule
ties it to `date` and `city`.)

The remaining fields are **refinable**. Concert details firm up over time — a
venue gets announced, a programme listed only by composer later names its works —
so `venue`, `program`, `pieces`, `country`, `location_tag`, and `source_url` may
be updated by a later run. The one limit is that information may not be *erased*:
a `venue`, `program`, or `pieces` that already carried a value may not be set
back to `null`. Detail can be added or corrected, never blanked out.

Extending the tag/domain allowlists, or clearing a field back to `null`, is a
reviewed change to this repo — not something the routine does on its own.

Run locally before committing (the validator reads `artists.json` from the
working directory too; `-artists ""` turns the roster checks off):

```sh
go test ./tools/...
go run ./tools/validate -file seen.json
```

## The artist roster (`artists.json`)

Which instrument a musician plays is a stable fact about the performer, not
something that varies per concert, so it is stored once in `artists.json`
rather than re-derived from concert pages on every run:

```json
{
  "artists": [
    { "slug": "fischer", "name": "Julia Fischer", "instruments": ["violin", "piano"] }
  ]
}
```

`slug` is the same slug used in a concert `id`, `name` must match the `artist`
string used in `seen.json` rows exactly, and `instruments` is a non-empty list
drawn from a closed vocabulary (`piano`, `violin`). CI validates the roster and
cross-checks it against `seen.json`, so an artist appearing in a concert row
without a roster entry — or under a subtly different name — fails the build.
`index.html` joins the two files to offer an instrument filter.

**The concert-watch routine never writes this file.** Adding an artist,
correcting an instrument, or extending the instrument vocabulary is a reviewed
change to this repo, exactly like extending the tag/domain allowlists. If a
concert turns up for an artist who is not on the roster, raise it rather than
editing the roster mid-run.

## Operating procedure for the concert-watch routine

You are a scheduled concert-monitoring agent. Your job: detect NEW upcoming
concerts by seven classical musicians and alert about them, using this repo as
memory so the same concert is never alerted on twice. You run inside a fresh
clone of this private repo with read/write access to repo contents and to
Issues. All state lives in `seen.json` at the repo root.

**Artists and primary sources:**

1. Olga Scheps — https://www.olgascheps.com/en/concerts/
2. María Dueñas — https://www.mariaduenasviolin.com/en/calendar
3. İlyun Bürkev — https://ilyunburkev.com/en/portfolio/concerts/
4. Maya Oganyan — https://www.mayaoganyan.com/calendar
5. Janine Jansen — https://www.janinejansen.com/performances/
6. Julia Fischer — https://www.juliafischer.com/en/events
7. Itzhak Perlman — https://itzhakperlman.com/performances/

All seven pages list upcoming concerts directly. Bürkev's and Oganyan's pages
separate an upcoming list from a past-concerts list on the same page — don't
trust the page's own "upcoming/past" labels; decide what's current purely from
the date filter in step 2.

**Secondary source — Bachtrack, for all seven artists too:**

1. Olga Scheps — https://bachtrack.com/performer/olga-scheps
2. María Dueñas — https://bachtrack.com/performer/maria-duenas
3. İlyun Bürkev — https://bachtrack.com/performer/ilyun-burkev
4. Maya Oganyan — https://bachtrack.com/performer/maya-oganyan
5. Janine Jansen — https://bachtrack.com/performer/janine-jansen
6. Julia Fischer — https://bachtrack.com/performer/julia-fischer
7. Itzhak Perlman — https://bachtrack.com/performer/itzhak-perlman

Each profile has a "Live Events" section listing upcoming concerts (ignore
"Latest reviews"/"Latest articles" — past content). Bachtrack sometimes lists
engagements before they appear on the artist's own site, so treat it as a
genuine cross-check, not a formality. If a profile URL 404s or the slug has
changed, try `https://bachtrack.com/search-events/performer=<slug>` as a
fallback before giving up on that artist's Bachtrack check.

**Step 1 — Load state.** Read `seen.json`. If it's missing or empty, this is
the first run: initialize it as `{"concerts": []}` and record everything found
below rather than treating the current slate as noise.

**Step 2 — Gather current concerts.** Determine today's date at runtime. For
each artist: fetch the primary source and the Bachtrack profile, and extract
every listed concert (ignore cookie banners, nav, and other page chrome).
For each, capture `artist`, `date`, `city`, `country`, `venue`, `program` (if
shown), `pieces` (per grounding rule 3), and the `source_url` you found it on.
Normalize dates to ISO `YYYY-MM-DD`; pages use mixed German/English formats
(`16.3.2026`, `04. Juni 2026`, `Aug 3, 2026`).

Keep only concerts dated today or later — discard past dates. If you can
access none of an artist's sources (official site AND Bachtrack), skip that
artist this run and note it in the step 7 report. If only some sources are
reachable, update using whichever succeeded. Aside from the official sites and
Bachtrack, don't reach for other sources — no general web searches, no
ticketing sites.

Do not invent concerts. Every concert must trace to a real `source_url` you
actually fetched this run. If a source fails to load, note it and move on —
don't guess its contents, and don't abort the whole run over one failed source.

**Step 3 — Tag location.** Set `location_tag` to:
- `"berlin"` — in Berlin or its immediate surroundings
- `"germany"` — elsewhere in Germany (reachable by regional rail on a
  Deutschlandticket, even if slow)
- `"europe"` — outside Germany but in Europe

Ignore events outside Europe.

**Step 4 — Deduplicate against memory.** Build a stable id per concert:
`id = "<artist-slug>|<ISO-date>|<city-lowercased>"` (e.g.
`"duenas|2026-08-03|berlin"`). A concert is NEW only if its id isn't already
in `seen.json`; tolerate minor venue/spelling differences so formatting
changes alone don't trigger a false alert. The same concert often appears on
both the official site and Bachtrack — since the id is based on artist/date/
city, it naturally collapses into one entry; don't record or alert on it
twice.

**Step 5 — Refine existing rows.** For every concert whose id is already in
`seen.json`, check whether this run's fetch turned up more than what's
stored — an announced venue, or real work titles where `pieces` previously
said `"Composers only: ..."` or `"Programme not announced"`. Update `venue`,
`program`, `pieces`, `country`, `location_tag`, and `source_url` in place per
grounding rule 6. Never touch `artist`, `date`, `city`, or `first_seen`, and
never clear a populated field back to `null`.

**Step 6 — Record and alert.** Use the `main` branch only — do not create new
branches. Add every NEW concert to `seen.json`'s `"concerts"` array with all
captured fields (including `pieces`) plus `"first_seen": "<today's ISO date>"`
and `"id"`. Commit with message `"concert-watch: <today's date>, +<N> new"`
and push.

If there's at least one NEW concert, open ONE GitHub issue:
- Title: `"New concerts found — <today's date> (<N> new)"`
- Body: group by `location_tag`, berlin first, then germany, then abroad. For
  each: `artist — date — city, venue — programme — source_url`.

If there are zero new concerts, do NOT open an issue — print a one-line
summary instead (e.g. "No new concerts. Checked 7 artists, all sources OK.").

**Step 7 — Report source health.** End the run output with a status line per
artist covering both sources: which of (official site, Bachtrack) loaded,
which failed, and how many concerts each currently contributed. This surfaces
a silently broken source.

## Grounding rules for the concert-watch routine

1. **Fetch before writing.** Every field must come from text actually returned by
   fetching that row's `source_url` during the run — never from memory or
   inference. If you didn't fetch it, don't record it.
2. **Null over guessing.** If `venue` or `program` isn't stated on the source
   page, leave it `null`. Never invent a venue, conductor, opus number, or
   program.
3. **`pieces` is a list only when the source lists works.** Record `pieces` as an
   array holding exactly the works named on the page, one per element, copied as
   stated:

   ```json
   "pieces": ["Chopin Ballade No. 1", "Chopin Ballade No. 2"]
   ```

   Do not add an opus/KV number, key, or nickname the page didn't print — if it
   says "Ballades Nos. 1 & 2", that is two elements, not a chance to supply
   Op. 23. When the page names no works, the field takes a plain string instead
   of an array, so an unknown programme is stated rather than fabricated:

   - `"Programme not announced"` — no repertoire given at all;
   - `"Composers only: Brahms, Schubert"` — composers named but no works.

   A composer's name is not a piece; never put one in the array to avoid writing
   the string form. `pieces` is required on every row — an empty array is
   rejected precisely so that "we don't know" has to be said out loud.
4. **Allowlisted sources only.** Only pull from the official artist calendars and
   Bachtrack (the domains listed above). Do not follow links to secondary
   aggregators.
5. **Verify new rows.** After drafting new entries, re-fetch each `source_url` and
   confirm the artist + date pair appears on the page before appending. Drop
   anything you cannot confirm.
6. **Refine, don't rewrite history.** Add new concerts, and update an existing
   row when the source now says more than it did — filling in an announced venue,
   or replacing `"Composers only: Lalo, Stravinsky"` with the actual works. Such
   an update is subject to rule 1 like any other write: it must come from text
   fetched in that run, not from what you happen to know about the repertoire.
   Never change `artist`, `date`, `city`, or `first_seen`, and never clear a
   populated field back to `null` — raise that as a normal PR for review.

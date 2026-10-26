# concerts-tracking

`seen.json` is a curated dataset of upcoming classical concerts, populated by an
automated concert-watch routine and rendered by `index.html` (GitHub Pages).
`artists.json` is the hand-maintained roster of the musicians tracked and the
instrument(s) each one plays — see "The artist roster" below. `favorites.json`
is the hand-maintained list of works worth travelling for, which the page marks
and the run alerts on — see "The favorites list" below. `travel.json` holds,
for each German city a concert is in, the approximate train time from Berlin
(from the Rome2Rio connector) and the city's station (from
`tools/stations/de.csv`), which the routine fills in and the page turns into a
travel line and a bahn.de link — see "Train times from Berlin" below.

Reducing hallucination in this dataset relies on three layers: the **enforced
layer**, which is what actually gates the data (CI); the **operating
procedure**, which is the routine's run script; and the **grounding rules**,
the principles that procedure leans on to avoid producing bad rows in the first
place. This file covers the data and the enforced layer, which every session
needs. The procedure and the grounding rules are in `ROUTINE.md`, which the
scheduled routine is told to read; step and rule numbers in this file refer to
it.

CI still decides which sites a row may *cite*: `source_url` has to sit on the
domain allowlist below, so a fabricated concert attributed to some invented
site fails the build on its hostname. What it cannot check is everything the
routine *reads*: the run now follows links onto promoter, venue and festival
pages no list vets, and the venue, works and cancellations they contribute
enter the data ungated. That weight sits on the procedure and the rules —
above all on corroboration (rules 5 and 10) and on copying rather than
recalling (rules 1 and 3). Read them as load-bearing, not advisory.

## Enforced layer (CI — cannot be hallucinated past)

`tools/validate` runs in CI (`.github/workflows/validate.yml`) on every change to
`seen.json` — on pushes to `main` and on every pull request, so the routine's own
run PR (step 7) is checked before a human merges it. It fails the build when an
entry:

- is missing a required field, or has an unknown/extra field;
- has a `date` or `first_seen` that is not a real zero-padded `YYYY-MM-DD`;
- has a `date` before its `first_seen` or more than three years after it, or a
  `first_seen` later than tomorrow (catches typoed years; measured from
  `first_seen` rather than today, so a row never ages out of validity);
- has a `location_tag` outside the allowed set (`europe`, `germany`, `berlin`);
- has a `source_url` that is not an absolute http(s) link on an allowlisted
  domain — the artist sites, `bachtrack.com`, and the label tour pages under
  "Tertiary source" in `ROUTINE.md`; or a `detail_url` that is present and is
  not an absolute http(s) link — `detail_url` is deliberately not restricted by
  domain, since the pages that carry the detail are exactly the ones no list
  vets (see "Where a row may come from" below);
- has a `status` outside the allowed set (`cancelled`, `postponed`,
  `artist_replaced`), or a `status` without a `status_note` (or a note without a
  status) — the two always travel together;
- has a `pieces` that is absent, an empty array, an empty string, or any shape
  other than an array of non-empty strings or a single non-empty string;
- has an `id` whose shape isn't `<slug>|<date>|<city>` or whose date/city
  segments disagree with the row's own fields;
- duplicates another entry's `id`;
- shares its `artist` and `date` with another entry, unless one of the two
  carries a `status` — two listings of one concert under different spellings of
  its city (`Luzern`, `Lucerne`) would otherwise become two permanent rows;
- names an artist absent from `artists.json`, or whose `artist` string disagrees
  with the name registered there for that id slug;
- is new (absent from the base) and belongs to an artist `artists.json` marks
  `"tracked": false`;
- has an `instruments` that is an empty array, repeats a value, contains
  anything outside the allowed set (`piano`, `violin`), or names an instrument
  the artist isn't recorded as playing in `artists.json`.

It also constrains how existing entries may change. Entries may never be
**deleted**, and the fields that pin a row to one specific concert — `artist`,
`date`, `city` — plus its provenance, `first_seen`, are **frozen** once written:
rewriting one of those would quietly repoint a vetted row at a different event
while keeping its id. (`id` itself is the identity key, and the id-shape rule
ties it to `date` and `city`.)

The remaining fields are **refinable**. Concert details firm up over time — a
venue gets announced, a programme listed only by composer later names its works —
so `venue`, `program`, `pieces`, `instruments`, `country`, `location_tag`,
`source_url`, `detail_url`, `status`, and `status_note` may be updated by a
later run. The one limit is that information may not be *erased*: a `venue`,
`program`, `pieces`, `instruments`, `detail_url`, `status`, or `status_note`
that already carried a value may not be set back to `null`, and a `pieces` that
lists works may not fall back to the string form. Detail can be added or
corrected, never blanked out — a `status` may be corrected to another value as
a source firms up, but dropping it, which would quietly put a called-off
concert back on the page, is a reviewed change like any other erasure.

Extending the `location_tag` or `instruments` vocabularies or the `source_url`
domain allowlist, or clearing a field back to `null`, is a reviewed change to
this repo — not something the routine does on its own.

`tools/validate` also checks `favorites.json` — well-formed slugs, titles and
patterns — though nothing in `seen.json` refers to that file, so there is no
per-row check to fail. Whether a concert plays a favorite is derived from the
row's `pieces` and the curated list whenever it is needed, never written into
the row.

It checks `travel.json` too. Every entry needs a `city` that some row in
`seen.json` spells exactly that way, so a misspelt city, which the page could
never match, fails the build. (Any row, not just a `germany` one: a row's
`location_tag` may be refined later, while its `city` is frozen and entries
are never removed, so tying the check to the tag could turn a retag into a
red build.) It also needs a `station`, a `train` or
both:
- a `station` must be a name-and-number pair listed in `tools/stations/de.csv`,
  so a station number recalled rather than looked up fails;
- a `train` needs all its fields, a duration of 1 to 1440 minutes, a real
  `checked` date, a `route` Rome2Rio names as a train route (it starts with
  `Train`, so a `Drive`, `Fly …` or `Night train` line copied by mistake
  fails), and a non-empty `carriers` list with no blank or repeated name.

Run locally before committing (the validator reads `artists.json`,
`favorites.json`, `travel.json` and `tools/stations/de.csv` from the working
directory too; `-artists ""`, `-favorites ""`, `-travel ""` and `-stations ""`
turn those checks off):

```sh
go test ./tools/...
go run ./tools/validate -file seen.json

# which upcoming concerts play a favorite (add -base to mark what is news)
go run ./tools/validate -file seen.json -favorites-report

# which station a German place name resolves to (step 6a)
go run ./tools/validate -find-station "Bad Wörishofen"
```

A second workflow (`.github/workflows/smoke.yml`) runs `tests/smoke.test.mjs`,
a small Playwright suite that loads `index.html` in a headless browser and
checks that the page comes up, renders its dataset, filters and searches, and
reports a load failure instead of hanging — plus one pass over the real
`seen.json` asserting the browser logged nothing. They are smoke tests: they
catch a page that has stopped working, not a subtly wrong one, and they say
nothing about whether the data is right, which is the validator's job above.

The one exception is favorite-matching, which the page and the routine
implement separately: the suite runs the page's matcher over
`tools/favorites/testdata/cases.json` and requires the same answers the Go tests
get from that file, so the two cannot quietly drift apart.

```sh
cd tests && npm install && npx playwright install chromium && npm test
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

The roster says what an artist *can* play; a single concert may call for only
one of those. So a `seen.json` row carries its own optional `instruments` —
`["piano"]` on a Fischer date the source describes as a piano recital — which
must be a subset of that artist's roster instruments. It is `null` whenever the
source doesn't say, which is the normal case and not a defect: the page then
treats the concert as a candidate for every instrument the artist plays, so an
unstated Fischer date appears under both Piano and Violin rather than claiming
one. A later run may narrow it once a source settles the question (rule 6) —
either by billing the instrument outright or by printing it in a work title,
which rule 7 admits as the one permitted inference.

**Dropping an artist** is done by marking the entry `"tracked": false`, never
by deleting it: rows are never removed from `seen.json`, so the artist's old
rows still need an entry to validate against (and their site stays on the
`source_url` allowlist for the same reason). The page hides every row of an
untracked artist, the favorites report skips them, and CI rejects any row
added for them after the flag is set. The field is either absent or `false`;
writing `true` fails the build. Also take the artist out of the source lists in
`ROUTINE.md` (the starting points, Bachtrack, and any label page).

**The concert-watch routine never writes this file.** Adding an artist,
correcting an instrument, or extending the instrument vocabulary is a reviewed
change to this repo, exactly like extending the `location_tag` vocabulary. If a
concert turns up for an artist who is not on the roster, raise it rather than
editing the roster mid-run.

## The favorites list (`favorites.json`)

Which works are worth travelling for is a fact about the reader, not about any
concert, so — like the roster — it is curated once in its own file:

```json
{
  "favorites": [
    {
      "slug": "chopin-ballade-1",
      "title": "Chopin — Ballade No. 1, Op. 23",
      "patterns": [
        ["chopin", "ballade", "no 1"],
        ["chopin", "ballade", "nr 1"],
        ["ballade", "op 23"]
      ]
    }
  ]
}
```

`slug` identifies the entry (kebab-case, since a work needs composer and number
to be identifiable), and `title` is what a human reads — the work as *you* would
name it. Neither is ever matched against. `patterns` does that.

**Why a work isn't identified by its title.** Sources phrase the same work a
dozen ways, and rule 3 says to copy whatever the page printed rather than
tidying it: `"Chopin Ballade No. 1"`, `"Ballade Nr. 1 g-Moll op. 23"`,
`"Chopin: Fantasie f-Moll"`, `"Concerto per violino e orchestra in re maggiore
op. 77"`. No single canonical string matches those. So a favorite carries the
phrasings that identify it, and a person curates them.

**The matching rule.** A favorite matches a work when every term of any one
pattern appears in it — an OR of ANDs. Both sides are normalized first
(lowercased, accents folded, punctuation turned into token boundaries, so
`Max Bruch: Violinkonzert Nr. 1 g-Moll op. 26` becomes `max bruch violinkonzert
nr 1 g moll op 26`), and a term matches only on whole tokens, so `op 2` never
matches `Op. 23`. Include the composer in a pattern unless an opus number
already makes the work unambiguous — `["ballade", "op 23"]` is safe,
`["ballade", "no 1"]` would catch Brahms.

**Writing a pattern that doesn't over-match.** An opus number identifies a work
only within a composer: `seen.json` carries both Beethoven's Op. 61 and
Saint-Saëns' Violin Concerto No. 3, Op. 61. So before trusting a pattern, read
it against the composer's *other* works — the ones that share its words:

- A work-type pair is not enough on its own. `["beethoven", "violin",
  "concerto"]` also matches "Triple Concerto for Violin, Cello and Piano", which
  is a real row. Add the key or the opus: `["beethoven", "violin", "concerto",
  "d major"]`.
- A number is only a discriminator where sources agree on it. Mendelssohn wrote
  two violin concertos, and listings call the E minor either "No. 1" or "No. 2"
  depending on whether they count the early D minor — so that favorite is
  matched on the key and Op. 64, never on a number, and the D minor stays out.
- Terms are matched as tokens, not as a phrase, so `["piano", "concerto"]` also
  catches "Concerto for Piano and Orchestra" and "Concerto per pianoforte"
  where the bigram `"piano concerto"` would not. Prefer the separate tokens,
  and lean on `[composer, "op NN"]` for listings in French, Italian or Spanish:
  the opus survives translation where the work's name doesn't.
- A catalogue number needs both spacings. These sources write `D 956` and
  `D956`, `BWV 1004` and `BWV861` — and `"d 957"` is two tokens, so it does not
  match `D957`. Any pattern anchored on a D, S, L, CD, BWV or KV number carries
  the fused form alongside the spaced one. (An opus escapes this: every listing
  seen so far writes `Op.` or `op. ` with a separator, so `"op 61"` is enough.)

Accents and case need no help — both sides are folded before comparison, so
`["schubert", "serenade"]` already matches "Sérénade" and a pattern spelled
with the accent is the same pattern written twice.

Matching is deliberately literal, and knows nothing about the repertoire. It
cannot decide that `"Beethoven Op. 61"` is the violin concerto, because that is
knowledge from outside the fetched page — exactly what the rest of this file
exists to keep out. A phrasing the patterns don't cover is a miss, and the fix
is a person adding a pattern.

**Only the array form of `pieces` is matched.** The string form —
`"Programme not announced"`, `"Composers only: Chopin"` — says the works are
unknown, and reading a favorite out of it would turn "we don't know" into
"your piece is on the bill".

**Nothing is written into `seen.json`.** Whether a concert plays a favorite is
a function of the row's `pieces` and this list, so it is computed where it is
needed — by `index.html` when it renders, by `tools/validate -favorites-report`
when a run alerts. A `favorites` field on a row would be one more field a run
could get wrong, gating nothing the two inputs don't already gate, and it would
go stale the moment the list changed. Deriving it also means starring a new work
lights up every concert already in the dataset: after editing this file, run the
report without `-base` to see everything it now catches.

**The concert-watch routine never writes this file**, exactly like the roster,
and it never decides a match by ear either. Step 7 runs the report and copies
what it says. If a programme looks to you like a favorite the report didn't
flag, that is a pattern a person should add: name it in the step 8 report and
leave the file alone.

The rule has two implementations — the Go matcher in `tools/favorites`, used by
the report, and a JavaScript mirror in `index.html`, used by the page — held to
the same answers by the shared fixture `tools/favorites/testdata/cases.json`.
Changing one means changing the other and adding a case there.

## Train times from Berlin (`travel.json`)

Every card for a concert elsewhere in Germany helps the reader judge the trip
from Berlin without opening a planner:

`≈ 4h 05m by train from Berlin · Deutschlandticket route ↗`

These are facts about the city, not the concert, so they are stored once per
city rather than on each row:

```json
{
  "cities": [
    {
      "city": "Kempen",
      "station": { "name": "Kempen (Niederrhein)", "eva": "8000409" },
      "train": {
        "query": "Kempen, Germany",
        "minutes": 333,
        "route": "Train via Wolfsburg",
        "carriers": ["Deutsche Bahn Intercity (DB IC)"],
        "checked": "2026-09-27"
      }
    }
  ]
}
```

- `city` is the concert's `city` exactly as `seen.json` spells it; the page
  joins on it, and CI rejects a city no row has.
- `station` is the city's railway station, as the name and DB station number
  (its EVA number) listed in `tools/stations/de.csv`, found with
  `-find-station`. It is left out when there is no clear answer; step 6a says
  when that is.
- `train` is the quickest *train* route the Rome2Rio connector returned from
  Berlin Hbf: `query` is the destination string sent, which may be fuller than
  `city` when the name alone is ambiguous (`"Frankfurt am Main, Germany"`);
  `minutes`, `route` and `carriers` are the route's duration, name and
  operators, copied as returned; and `checked` is the day the query ran.

Either half may be missing, for example when the connector wasn't available
to a run, or when the place has no station of its own. Both missing means no
entry at all. Berlin cards show nothing, and neither do concerts abroad, or a
German city nobody has filled in yet.

**What the page shows.**
- **The time**, from `train`, rounded to five minutes and marked "≈". What
  Rome2Rio returns is a typical duration for a route, not a timetable, and the
  tooltip names the route and the day it was checked.
- **A ✓ Deutschlandticket badge**, when every one of the route's `carriers` is
  on the page's list of regional operators (`REGIONAL_CARRIERS` in
  `index.html`). In practice that means cities where regional trains are the
  fastest, such as Rostock. It is worked out when the page loads, never
  stored, like a favorite. The list names what is known to be regional, so an
  operator nobody has added yet costs a route its badge and never makes an ICE
  look covered. Extending it is a reviewed change to `index.html`, not
  something a run does.
- **A Deutschlandticket route ↗ link**, from `station`: an int.bahn.de search
  from Berlin Hbf, arriving by 18:00 on the concert day, with
  Deutschlandticket connections only. bahn.de ignores a destination given by
  name alone and resets the whole search, date included, so a city without a
  `station` gets no link. The link's shape was taken from a search a person
  made and shared on int.bahn.de, and on 2026-09-27 a person confirmed that
  the trimmed form opens on the right day with regional trains only. bahn.de
  blocks automated requests, so no test can re-check that; a person does, by
  clicking.

The time exists because Rome2Rio lists only its top four routes, so a slower
regional-only route almost never comes back: there is no regional time to
store, and the link stands in for one.

**Unlike the roster and the favorites, the routine writes this file.** Step 6a
says how. It adds entries, fills in a missing half, and refreshes a stale
`train`; it never removes an entry. A wrong entry is corrected by a person
like any other reviewed change.

**Where the numbers come from.**
- **Train times:** only from the Rome2Rio connector's `get-routes` tool. The
  web alternatives failed from the cloud container: bahn.de's API and
  rome2rio.com itself refused automated requests (an Akamai bot block and a
  Cloudflare challenge), and the community `db.transport.rest` API returned
  503. The connector goes through Rome2Rio's API instead.
- **Stations:** only from `tools/stations/de.csv`, the German part of the
  open trainline-eu/stations dataset (ODbL; see `tools/stations/README.md`).
  It is checked into the repo, so the lookup needs no network and CI can
  check every station against it.

Like every negative finding in this file, those refusals are one day's result
(rule 11). Still, don't route around a bot block to get a number: no faked
browser headers, no headless browser. When the connector is not available to
the run, the `train` halves wait for a run where it is.

## Where a row may come from (`source_url` vs `detail_url`)

Calendar pages are terse. Most entries give a date, a city and — if you are
lucky — a venue, while the programme sits a click or two away on the pages the
entry links to. Olga Scheps' calendar announced a Kempen recital with no hall at
all; the promoter it links to names one in its first paragraph. María Dueñas'
calendar is the sharper version: it names almost no repertoire anywhere, so what
it links to is all there is to read. A festival's own page routinely prints a
fuller bill than any aggregator carries for the same night.

So the routine follows links, and follows them further than it used to. **No
domain list decides what may be *read*** — but one still decides what a row may
*cite*: `source_url` must be on the allowlist, `detail_url` need not be. A
promoter page reached by following a link can fill in the venue and the works,
while the row still traces back to the swept source that stated the concert.

For everything such a page contributes, the defence is not the hostname but
evidence on the page: it says nothing until it shows this artist on this date,
and the row records which page that was. That check (rules 5 and 10), the
freeze on `artist`/`date`/`city`, treating every page as data rather than
instructions (rule 8), and copying rather than recalling (rules 1 and 3) are
what carry that weight.

A row carries two links, answering different questions:

- **`source_url` — the page that stated this concert.** The page whose words
  establish that the engagement exists: an artist's calendar, a Bachtrack
  listing, or a label tour page — one of the swept sources, since it has to be
  on the allowlist. It must name the artist and the date (rule 5).
- **`detail_url` — the concert's own page, the best one reached.** Where the
  detail came from: typically the promoter, venue or ticket page. It is `null`
  when nothing followable was offered, when what was offered could not be
  fetched, or when the listing already said everything. It is also `null` when
  the page that stated the concert is itself the page with the detail — a
  festival page that both bills the date and prints the works is one link, not
  two, and it belongs in `source_url`.

`detail_url` also keeps the dataset auditable. Once a programme comes from the
promoter's page rather than the calendar, "re-read `source_url` and check" no
longer reaches the text the row was built from — recording the page that did
say it puts that back.

## When a concert falls through (`status`)

Rows are never deleted and a row's `date` is frozen, so the dataset cannot say
"don't go to that one" by removing or moving anything. `status` is how it says
so out loud, drawn from a closed vocabulary:

- `"cancelled"` — the concert is not happening.
- `"postponed"` — moved to a date this row cannot represent. If the new date is
  announced, it arrives as a NEW concert with its own row; this row still
  records what became of the old date.
- `"artist_replaced"` — the concert goes ahead, but the artist we track is not
  playing it: another soloist is billed, or their appearance is off.

`status_note` travels with it and quotes what the source actually said —
`Tivoli: the concert is cancelled due to illness.` CI rejects one without the
other, so a status is never a bare assertion, and the page prints the note
beside a struck-through billing rather than hiding the row.

Two rules make this safe to automate:

- **Only a source that says it.** A concert disappearing from a calendar is not
  a cancellation: sites paginate, re-sort, drop past events, and rebuild.
  Silence is not evidence, and neither is a page that merely fails to load.
- **Expect the news on the detail page, not the artist's calendar.** When this
  step was written all three of Janine Jansen's late-August 2026 dates were
  still listed on janinejansen.com while Berwaldhallen said Karen Gomyo had
  replaced her, the Helsinki Festival said her appearance was cancelled, and
  Tivoli said the concert was off. The promoter knows first.

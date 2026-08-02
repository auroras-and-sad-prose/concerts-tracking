# concerts-tracking

`seen.json` is a curated dataset of upcoming classical concerts, populated by an
automated concert-watch routine and rendered by `index.html` (GitHub Pages).

Reducing hallucination in this dataset relies on two layers. The enforced layer
below is what actually gates the data; the grounding rules are what the routine
should follow to keep from producing bad rows in the first place.

## Enforced layer (CI — cannot be hallucinated past)

`tools/validate` runs in CI (`.github/workflows/validate.yml`) on every change to
`seen.json`. It fails the build when an entry:

- is missing a required field, or has an unknown/extra field;
- has a `date` or `first_seen` that is not a real zero-padded `YYYY-MM-DD`;
- has a concert year outside `now-1 .. now+3` (catches typoed years);
- has a `location_tag` outside the allowed set (`europe`, `germany`, `berlin`);
- has a `source_url` that is not http(s) on an allowlisted domain
  (`ilyunburkev.com`, `olgascheps.com`, `mariaduenasviolin.com`,
  `mayaoganyan.com`, `bachtrack.com`);
- has a `pieces` that is absent, an empty array, an empty string, or any shape
  other than an array of non-empty strings or a single non-empty string;
- has an `id` whose shape isn't `<slug>|<date>|<city>` or whose date/city
  segments disagree with the row's own fields;
- duplicates another entry's `id`.

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

Run locally before committing:

```sh
go test ./tools/...
go run ./tools/validate -file seen.json
```

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

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
  (`ilyunburkev.com`, `olgascheps.com`, `mariaduenasviolin.com`, `bachtrack.com`);
- has an `id` whose shape isn't `<slug>|<date>|<city>` or whose date/city
  segments disagree with the row's own fields;
- duplicates another entry's `id`.

It also enforces **append-only**: existing entries may not be modified or
deleted, only new ones added. This keeps a bad generation from corrupting rows
that were already vetted. Extending the tag/domain allowlists, or making a
deliberate correction to an existing row, is a reviewed change to this repo — not
something the routine does on its own.

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
3. **Allowlisted sources only.** Only pull from the official artist calendars and
   Bachtrack (the domains listed above). Do not follow links to secondary
   aggregators.
4. **Verify new rows.** After drafting new entries, re-fetch each `source_url` and
   confirm the artist + date pair appears on the page before appending. Drop
   anything you cannot confirm.
5. **Append only.** Add new concerts; never rewrite or remove existing entries. A
   genuine correction should be raised as a normal PR for review.

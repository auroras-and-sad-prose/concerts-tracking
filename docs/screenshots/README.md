# UI snapshots

What `index.html` looked like when a change to it went up for review. Each set
belongs to one change and is **not** refreshed afterwards, so read a set as a
record of that change rather than as a picture of the page today — the
`pieces-programme-*` shots below already predate the current masthead, the
instrument filter and the Details links.

Nothing loads these at runtime; they exist so a reviewer can see a visual change
without checking the branch out.

| Files | Change |
| --- | --- |
| `pieces-programme-{light,dark,search}.png` | The `Programme` label and work chips, and the search box matching on a composer. |
| `favorites-{light,dark}.png` | The favorites filter set to *Any favorite*, with the gold star on the matched work and on the card carrying it. |
| `favorites-in-context.png` | Unfiltered, so a starred card sits next to an unstarred one and a flagged one — gold edge against red. |
| `themes-{classic,swiss,poster,calendar,tickets,lanes,listings}.png` | The theme picker: one shot per theme, light scheme, real `seen.json`. `classic` is the default and the only theme with a dark variant. |

To take a new set, serve the repo root over HTTP (the page fetches `seen.json`,
`artists.json` and `favorites.json` relative to itself, so `file://` won't do —
`tests/smoke.test.mjs` has a `startServer` that does this) and screenshot the
page at 900px wide in each colour scheme. Keep them at 1× rather than 2×: these
are for reading, and the repo carries them forever.

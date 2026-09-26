// Smoke tests for index.html — does the page come up, render the dataset, and
// respond to its controls without throwing? They deliberately stop there: the
// dataset's own integrity is CI's job (tools/validate), and these tests only
// check that the page which renders it doesn't fall over.
//
// Run from this directory:
//
//   npm install
//   npx playwright install chromium   # once per Playwright version
//   npm test
//
// Chromium goes to Playwright's usual browser location; set
// PLAYWRIGHT_BROWSERS_PATH if the machine keeps it elsewhere.

import { after, before, describe, test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const REPO_ROOT = fileURLToPath(new URL("..", import.meta.url));

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".png": "image/png",
};

// The page fetches seen.json and artists.json relative to itself, so it needs a
// real origin rather than a file:// URL.
async function startServer() {
  const server = createServer(async (req, res) => {
    const path = normalize(decodeURIComponent(new URL(req.url, "http://x").pathname));
    const file = join(REPO_ROOT, path === "/" ? "index.html" : path);
    if (!file.startsWith(REPO_ROOT)) {
      res.writeHead(403).end();
      return;
    }
    try {
      const body = await readFile(file);
      res.writeHead(200, { "content-type": MIME[extname(file)] || "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404, { "content-type": "text/plain" }).end("not found");
    }
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  return { server, origin: `http://127.0.0.1:${server.address().port}` };
}

const iso = daysFromToday => {
  const d = new Date();
  d.setDate(d.getDate() + daysFromToday);
  return d.toISOString().slice(0, 10);
};

// Dates are relative to the run so the fixture never ages out of the page's
// "today or later" filter.
const PAST = iso(-30);
const SOON = iso(30);
const LATER = iso(400);

const FIXTURE_ARTISTS = {
  artists: [
    { slug: "fischer", name: "Julia Fischer", instruments: ["violin", "piano"] },
    { slug: "scheps", name: "Olga Scheps", instruments: ["piano"] },
  ],
};

// One favorite the fixture programmes play, one named only by a row whose
// pieces are the string form (which must never match), and one nothing plays.
const FIXTURE_FAVORITES = {
  favorites: [
    {
      slug: "brahms-violin-concerto",
      title: "Brahms — Violin Concerto in D major, Op. 77",
      patterns: [["brahms", "violin concerto"], ["brahms", "op 77"]],
    },
    { slug: "mozart-any", title: "Anything by Mozart", patterns: [["mozart"]] },
    {
      slug: "chopin-ballade-1",
      title: "Chopin — Ballade No. 1 in G minor, Op. 23",
      patterns: [["chopin", "ballade", "no 1"]],
    },
  ],
};

// Kempen has a German concert in the fixture; Berlin is there too, to show a
// Berlin card never gets a train time even when the file has one for it.
// 328 minutes is shown rounded to five: "5h 30m".
const FIXTURE_TRAVEL = {
  cities: [
    { city: "Kempen", query: "Kempen, Germany", minutes: 328, route: "Train via Wolfsburg", checked: PAST },
    { city: "Berlin", query: "Berlin, Germany", minutes: 10, route: "Train", checked: PAST },
  ],
};

const FIXTURE_CONCERTS = {
  concerts: [
    {
      id: `fischer|${SOON}|berlin`,
      artist: "Julia Fischer",
      date: SOON,
      city: "Berlin",
      country: "Germany",
      venue: "Philharmonie",
      program: "Berliner Philharmoniker",
      pieces: ["Brahms Violin Concerto in D major, Op. 77"],
      instruments: ["violin"],
      location_tag: "berlin",
      source_url: "https://www.juliafischer.com/en/events",
      detail_url: "https://example.org/philharmonie-event",
      first_seen: PAST,
    },
    {
      id: `scheps|${LATER}|kempen`,
      artist: "Olga Scheps",
      date: LATER,
      city: "Kempen",
      country: "Germany",
      venue: null,
      program: null,
      pieces: "Programme not announced",
      location_tag: "germany",
      source_url: "https://www.olgascheps.com/konzerte",
      first_seen: PAST,
    },
    {
      id: `fischer|${LATER}|vienna`,
      artist: "Julia Fischer",
      date: LATER,
      city: "Vienna",
      country: "Austria",
      venue: "Musikverein",
      program: null,
      pieces: "Composers only: Mozart",
      location_tag: "europe",
      source_url: "https://bachtrack.com/performer/julia-fischer",
      status: "cancelled",
      status_note: "Musikverein: the concert is cancelled due to illness.",
      first_seen: PAST,
    },
    {
      id: `scheps|${PAST}|hamburg`,
      artist: "Olga Scheps",
      date: PAST,
      city: "Hamburg",
      country: "Germany",
      venue: "Elbphilharmonie",
      program: null,
      pieces: "Programme not announced",
      location_tag: "germany",
      source_url: "https://www.olgascheps.com/konzerte",
      first_seen: PAST,
    },
  ],
};

let browser;
let origin;
let server;

before(async () => {
  browser = await chromium.launch();
  ({ server, origin } = await startServer());
});

after(async () => {
  await browser?.close();
  await new Promise(resolve => server.close(resolve));
});

// Opens index.html and returns the page plus everything it complained about.
// `data` swaps in fixture JSON; omit it to exercise the checked-in dataset.
// `query` is appended to the page URL, e.g. "?theme=calendar".
async function open(data = null, query = "") {
  const page = await browser.newPage();
  const errors = [];
  page.on("console", msg => msg.type() === "error" && errors.push(msg.text()));
  page.on("pageerror", err => errors.push(String(err)));

  // The webfonts are decoration and the tests shouldn't depend on reaching
  // Google, so they are answered locally with an empty stylesheet.
  await page.route("https://fonts.*/**", route =>
    route.fulfill({ status: 200, contentType: "text/css", body: "" }));

  if (data) {
    await page.route("**/seen.json", route =>
      route.fulfill({ json: data.concerts ?? FIXTURE_CONCERTS }));
    await page.route("**/artists.json", route =>
      route.fulfill({ json: data.artists ?? FIXTURE_ARTISTS }));
    await page.route("**/favorites.json", route =>
      route.fulfill({ json: data.favorites ?? FIXTURE_FAVORITES }));
    await page.route("**/travel.json", route =>
      route.fulfill({ json: data.travel ?? FIXTURE_TRAVEL }));
  }

  await page.goto(origin + "/" + query, { waitUntil: "networkidle" });
  return { page, errors };
}

describe("the concert page", () => {
  test("renders the checked-in dataset without erroring", async () => {
    const { page, errors } = await open();

    assert.equal(await page.title(), "Upcoming Concerts");
    assert.deepEqual(errors, []);
    // A dataset whose dates have all passed is legitimate, so the count is not
    // asserted — only that the page reported one instead of failing. The
    // favorites clause is likewise optional: whether the real dataset happens
    // to play one today is not this suite's business.
    assert.match(
      await page.locator("#subtitle").innerText(),
      /^\d+ upcoming concerts?( · \d+ with a favorite)?$/);
    await page.close();
  });

  test("lists upcoming concerts and leaves past ones out", async () => {
    const { page, errors } = await open({});

    assert.equal(await page.locator(".card").count(), 3);
    assert.equal(await page.locator("#subtitle").innerText(), "3 upcoming concerts · 1 with a favorite");
    assert.ok(await page.locator(".month-heading").count() >= 1);

    const text = await page.locator("#main").innerText();
    assert.match(text, /Julia Fischer/);
    assert.match(text, /Philharmonie/);
    assert.match(text, /Brahms Violin Concerto in D major, Op. 77/);
    assert.doesNotMatch(text, /Elbphilharmonie/);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("marks a concert that has been called off", async () => {
    const { page, errors } = await open({});

    const flagged = page.locator(".card.flagged");
    assert.equal(await flagged.count(), 1);
    assert.match(await flagged.innerText(), /cancelled due to illness/i);
    // The label is upper-cased in CSS, so match it case-insensitively.
    assert.match(await flagged.locator(".tag.status").innerText(), /^cancelled$/i);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("populates its filters and narrows the list", async () => {
    const { page, errors } = await open({});

    await page.selectOption("#artistFilter", "Olga Scheps");
    assert.equal(await page.locator(".card").count(), 1);
    assert.match(await page.locator(".card").innerText(), /Kempen/);

    // The instrument control appears only because the roster loaded and names
    // an artist who plays more than one.
    await page.selectOption("#artistFilter", "");
    assert.equal(await page.locator("#instrumentFilter").isHidden(), false);
    // Violin keeps both Fischer dates — one bills the violin outright, the
    // other states no instrument and so counts for everything she plays — and
    // drops the pianist.
    await page.selectOption("#instrumentFilter", "violin");
    assert.equal(await page.locator(".card").count(), 2);
    assert.doesNotMatch(await page.locator("#main").innerText(), /Kempen/);

    await page.selectOption("#instrumentFilter", "");
    await page.selectOption("#countryFilter", "Austria");
    assert.equal(await page.locator(".card").count(), 1);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("marks the works on the favorites list", async () => {
    const { page, errors } = await open({});

    const starred = page.locator(".card.favorite");
    assert.equal(await starred.count(), 1);
    assert.match(await starred.innerText(), /Julia Fischer/);
    assert.match(await starred.locator(".tag.favorite").innerText(), /Favorite/);

    // The individual work is marked, not the whole programme.
    assert.equal(await page.locator(".piece.favorite").count(), 1);
    assert.match(await page.locator(".piece.favorite").innerText(), /Brahms Violin Concerto/);
    assert.match(
      await page.locator(".piece.favorite").getAttribute("title"),
      /Brahms — Violin Concerto in D major, Op\. 77/);

    assert.deepEqual(errors, []);
    await page.close();
  });

  // "Composers only: Mozart" says the works are unknown. Reading a favorite
  // out of it would turn "we don't know" into "your piece is on the bill".
  test("never reads a favorite out of an unannounced programme", async () => {
    const { page, errors } = await open({});

    await page.selectOption("#favoriteFilter", "mozart-any");
    assert.equal(await page.locator(".card").count(), 0);
    assert.match(await page.locator(".empty").innerText(), /No upcoming concerts match/);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("filters by favorite work", async () => {
    const { page, errors } = await open({});

    assert.equal(await page.locator("#favoriteFilter").isHidden(), false);

    await page.selectOption("#favoriteFilter", "*");
    assert.equal(await page.locator(".card").count(), 1);
    assert.equal(await page.locator("#subtitle").innerText(), "1 upcoming concert · 1 with a favorite");

    await page.selectOption("#favoriteFilter", "brahms-violin-concerto");
    assert.equal(await page.locator(".card").count(), 1);

    // A favorite nothing on the calendar plays is still offered — that it
    // narrows to nothing is the answer.
    await page.selectOption("#favoriteFilter", "chopin-ballade-1");
    assert.equal(await page.locator(".card").count(), 0);

    await page.selectOption("#favoriteFilter", "");
    assert.equal(await page.locator(".card").count(), 3);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("shows the train time from Berlin on German cards only", async () => {
    const { page, errors } = await open({});

    const travel = page.locator(".travel");
    assert.equal(await travel.count(), 1);
    assert.equal(await travel.innerText(), "≈ 5h 30m by train from Berlin");
    assert.match(await travel.getAttribute("title"), /Train via Wolfsburg, 328 min/);
    const kempen = page.locator(".card", { hasText: "Kempen" });
    assert.equal(await kempen.locator(".travel").count(), 1);

    assert.deepEqual(errors, []);
    await page.close();
  });

  // The page highlights favorites and the concert-watch routine alerts on
  // them, from two implementations of one rule (index.html and
  // tools/favorites). This is the fixture that keeps them from drifting: if it
  // fails here, the page and the alerts have started disagreeing about what
  // counts as a favorite.
  test("matches favorites exactly as the Go implementation does", async () => {
    const { page, errors } = await open({});
    const cases = JSON.parse(
      await readFile(join(REPO_ROOT, "tools", "favorites", "testdata", "cases.json"), "utf8"));

    const normalized = await page.evaluate(
      inputs => inputs.map(s => normalizeTitle(s)),
      cases.normalize.map(c => c.in));
    assert.deepEqual(normalized, cases.normalize.map(c => c.out));

    const matched = await page.evaluate(({ favorites, pieces }) => {
      const parsed = parseFavorites(favorites);
      return pieces.map(piece => favoriteHitsIn([piece], parsed).map(h => h.slug));
    }, { favorites: cases.favorites, pieces: cases.match.map(c => c.piece) });
    assert.deepEqual(matched, cases.match.map(c => c.slugs));

    assert.deepEqual(errors, []);
    await page.close();
  });

  // Every theme restyles the same markup, so each must still show every row
  // along with what marks one out: the status of a called-off concert and the
  // starred work.
  test("renders the same concerts in every theme", async () => {
    const { page, errors } = await open({});

    assert.equal(await page.locator("html").getAttribute("data-theme"), "classic");
    const themes = await page.locator("#themePicker option").evaluateAll(opts => opts.map(o => o.value));
    assert.equal(themes.length, 7);
    assert.equal(themes[0], "classic");

    for (const theme of themes) {
      await page.selectOption("#themePicker", theme);
      assert.equal(await page.locator("html").getAttribute("data-theme"), theme);
      assert.equal(await page.locator(".card").count(), 3, theme);
      assert.equal(await page.locator(".card.flagged .tag.status").count(), 1, theme);
      assert.equal(await page.locator(".piece.favorite").count(), 1, theme);
      assert.equal(await page.locator("#subtitle").innerText(), "3 upcoming concerts · 1 with a favorite", theme);
      assert.match(await page.locator("#main").innerText(), /5h 30m/, theme);
    }

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("narrows the list to a day picked on the calendar", async () => {
    const { page, errors } = await open({}, "?theme=calendar");

    assert.equal(await page.locator("#viz").isHidden(), false);
    await page.click(`[data-day="${SOON}"]`);
    assert.equal(await page.locator(".card").count(), 1);
    assert.match(await page.locator(".card").innerText(), /Philharmonie/);

    await page.click("[data-cal-clear]");
    assert.equal(await page.locator(".card").count(), 3);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("draws one lane per rostered artist and one dot per concert", async () => {
    const { page, errors } = await open({}, "?theme=lanes");

    assert.equal(await page.locator(".lane-track").count(), FIXTURE_ARTISTS.artists.length);
    assert.equal(await page.locator(".lane-dot").count(), 3);
    await page.selectOption("#artistFilter", "Olga Scheps");
    assert.equal(await page.locator(".lane-dot").count(), 1);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("searches, and says so when nothing matches", async () => {
    const { page, errors } = await open({});

    await page.fill("#searchBox", "kempen");
    assert.equal(await page.locator(".card").count(), 1);

    await page.fill("#searchBox", "no such concert anywhere");
    assert.equal(await page.locator(".card").count(), 0);
    assert.match(await page.locator(".empty").innerText(), /No upcoming concerts match/);

    assert.deepEqual(errors, []);
    await page.close();
  });

  test("reports a failure to load seen.json instead of hanging on Loading…", async () => {
    const page = await browser.newPage();
    const crashes = [];
    page.on("pageerror", err => crashes.push(String(err)));
    await page.route("https://fonts.*/**", route =>
      route.fulfill({ status: 200, contentType: "text/css", body: "" }));
    await page.route("**/seen.json", route => route.fulfill({ status: 500, body: "" }));

    await page.goto(origin, { waitUntil: "networkidle" });

    assert.match(await page.locator("#main").innerText(), /Could not load seen\.json/);
    assert.equal(await page.locator("#subtitle").innerText(), "Error");
    assert.deepEqual(crashes, []);
    await page.close();
  });

  test("still lists concerts when the artist roster is missing", async () => {
    const page = await browser.newPage();
    const crashes = [];
    page.on("pageerror", err => crashes.push(String(err)));
    await page.route("https://fonts.*/**", route =>
      route.fulfill({ status: 200, contentType: "text/css", body: "" }));
    await page.route("**/seen.json", route => route.fulfill({ json: FIXTURE_CONCERTS }));
    await page.route("**/artists.json", route => route.fulfill({ status: 404, body: "" }));
    await page.route("**/favorites.json", route => route.fulfill({ json: FIXTURE_FAVORITES }));

    await page.goto(origin, { waitUntil: "networkidle" });

    assert.equal(await page.locator(".card").count(), 3);
    // Instrument tags come from the roster telling us the artist plays more
    // than one, so without it the cards simply carry none.
    assert.equal(await page.locator(".tag.instrument").count(), 0);
    assert.deepEqual(crashes, []);
    await page.close();
  });

  test("still lists concerts when the favorites list is missing", async () => {
    const page = await browser.newPage();
    const crashes = [];
    page.on("pageerror", err => crashes.push(String(err)));
    await page.route("https://fonts.*/**", route =>
      route.fulfill({ status: 200, contentType: "text/css", body: "" }));
    await page.route("**/seen.json", route => route.fulfill({ json: FIXTURE_CONCERTS }));
    await page.route("**/artists.json", route => route.fulfill({ json: FIXTURE_ARTISTS }));
    await page.route("**/favorites.json", route => route.fulfill({ status: 404, body: "" }));

    await page.goto(origin, { waitUntil: "networkidle" });

    // Nothing is starred and the control stays hidden rather than offering a
    // filter with nothing behind it.
    assert.equal(await page.locator(".card").count(), 3);
    assert.equal(await page.locator(".card.favorite").count(), 0);
    assert.equal(await page.locator("#favoriteFilter").isHidden(), true);
    assert.equal(await page.locator("#subtitle").innerText(), "3 upcoming concerts");
    assert.deepEqual(crashes, []);
    await page.close();
  });

  test("still lists concerts when the train times are missing", async () => {
    const page = await browser.newPage();
    const crashes = [];
    page.on("pageerror", err => crashes.push(String(err)));
    await page.route("https://fonts.*/**", route =>
      route.fulfill({ status: 200, contentType: "text/css", body: "" }));
    await page.route("**/seen.json", route => route.fulfill({ json: FIXTURE_CONCERTS }));
    await page.route("**/artists.json", route => route.fulfill({ json: FIXTURE_ARTISTS }));
    await page.route("**/favorites.json", route => route.fulfill({ json: FIXTURE_FAVORITES }));
    await page.route("**/travel.json", route => route.fulfill({ status: 404, body: "" }));

    await page.goto(origin, { waitUntil: "networkidle" });

    assert.equal(await page.locator(".card").count(), 3);
    assert.equal(await page.locator(".travel").count(), 0);
    assert.deepEqual(crashes, []);
    await page.close();
  });
});

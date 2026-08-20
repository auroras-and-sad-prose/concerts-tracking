// Smoke tests for index.html — does the page come up, render the dataset, and
// respond to its controls without throwing? They deliberately stop there: the
// dataset's own integrity is CI's job (tools/validate), and these tests only
// check that the page which renders it doesn't fall over.
//
// Run from this directory:
//
//   npm install
//   npm test
//
// Chromium comes from Playwright's usual browser location; set
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
async function open(data = null) {
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
  }

  await page.goto(origin, { waitUntil: "networkidle" });
  return { page, errors };
}

describe("the concert page", () => {
  test("renders the checked-in dataset without erroring", async () => {
    const { page, errors } = await open();

    assert.equal(await page.title(), "Upcoming Concerts");
    assert.deepEqual(errors, []);
    // A dataset whose dates have all passed is legitimate, so the count is not
    // asserted — only that the page reported one instead of failing.
    assert.match(await page.locator("#subtitle").innerText(), /^\d+ upcoming concerts?$/);
    await page.close();
  });

  test("lists upcoming concerts and leaves past ones out", async () => {
    const { page, errors } = await open({});

    assert.equal(await page.locator(".card").count(), 3);
    assert.equal(await page.locator("#subtitle").innerText(), "3 upcoming concerts");
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

    await page.goto(origin, { waitUntil: "networkidle" });

    assert.equal(await page.locator(".card").count(), 3);
    // Instrument tags come from the roster telling us the artist plays more
    // than one, so without it the cards simply carry none.
    assert.equal(await page.locator(".tag.instrument").count(), 0);
    assert.deepEqual(crashes, []);
    await page.close();
  });
});

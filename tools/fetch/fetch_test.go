package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The placeholder the search page served on 2026-11-23.
const deferredPage = `<html><body><div class="btk-search-results">
<div class="btk-search-deferred" data-search-for="listing" data-criteria="performer=olga-scheps">Loading, please wait...</div>
</div></body></html>`

// listing is one listing as deferred-results sent it on 2026-11-23 (trimmed):
// a desktop and a mobile card, each in a wrapper carrying data-id, data-type
// and data-dates, with a wish-list button that repeats the id.
func listing(id string) string {
	return fmt.Sprintf(`<div data-id='%[1]s' data-type='nothing' data-fkmyevent='' data-dates='1802620800'><div class='listing-shortform data-sfdate=&#039;2026-11-20 10:56:46&#039; listing-medium-1 li-shortform-premium'><div class='listing-shortform-left'><div class='listing-shortform-dates'>Sun, 14 Feb at&nbsp;17:00</div>
<span data-id='%[1]s' data-dates='1802620800' class='myevent-listing-add-wishlist'>Wish list</span></div></div></div>
<div data-id='%[1]s' data-type='nothing' data-dates='1802620800'><div class='listing-ms'><div class='listing-ms-city'>Andermatt</div></div></div>
`, id)
}

func showing(n, total int) string {
	return fmt.Sprintf("<div class='btk-search-result-info'>Showing <span class='btk-search-result-count'>%d</span> of <span class='btk-search-result-total'>%d</span> listings</div>\n", n, total)
}

func TestTagsWithClass(t *testing.T) {
	doc := `<div class="a listing-ms-city" data-id="1"></div>
<li data-id='2' class="x listing-ms">
<span class=listing-ms data-param="a=b&amp;c=d">`
	got := tagsWithClass(doc, "listing-ms")
	if len(got) != 2 {
		t.Fatalf("got %d tags, want 2 (listing-ms-city is another class): %v", len(got), got)
	}
	if got[0]["data-id"] != "2" {
		t.Errorf("single-quoted attribute: got %q", got[0]["data-id"])
	}
	if got[1]["data-param"] != "a=b&c=d" {
		t.Errorf("entities not decoded: got %q", got[1]["data-param"])
	}
}

// A '>' inside a quoted attribute value must not end the tag: the class
// after it would be lost, and the listing with it.
func TestTagsWithClassQuotedGreaterThan(t *testing.T) {
	doc := `<a data-label='Bach > Brahms' class='listing-ms' data-id='7'>x</a><b title="1>0" class=listing-ms>`
	got := tagsWithClass(doc, "listing-ms")
	if len(got) != 2 || got[0]["data-id"] != "7" || got[1]["title"] != "1>0" {
		t.Fatalf("got %v", got)
	}
}

// A quote that does not open a value (alt=O'Brien) must not swallow the
// tags after it: here, the next listing's wrapper and card.
func TestTagsStrayApostrophe(t *testing.T) {
	doc := "<img alt=O'Brien src=x>" + listing("5") + `<a title='unterminated>`
	if n, desk, mob := countListings(doc); n != 1 || desk != 1 || mob != 1 {
		t.Fatalf("got %d listing(s), %d desktop, %d mobile; want 1, 1, 1", n, desk, mob)
	}
}

func TestDeferredAttrs(t *testing.T) {
	sf, cr, ok := deferredAttrs(deferredPage)
	if !ok || sf != "listing" || cr != "performer=olga-scheps" {
		t.Fatalf("got %q %q %v", sf, cr, ok)
	}
	if _, _, ok := deferredAttrs(`<div class="btk-search-results"></div>`); ok {
		t.Error("found a deferred element on a page without one")
	}
}

func TestSearchMore(t *testing.T) {
	if _, _, found, err := searchMore(listing("1")); found || err != nil {
		t.Errorf("no pager: found=%v err=%v", found, err)
	}
	n, p, found, err := searchMore(`<a class="btn btk-search-more" data-param="performer=x" data-startrow="20">See more</a>`)
	if !found || err != nil || n != 20 || p != "performer=x" {
		t.Errorf("pager: %d %q %v %v", n, p, found, err)
	}
	if _, _, found, err := searchMore(`<a class="btk-search-more" data-startrow="">`); !found || err == nil {
		t.Errorf("unusable startrow: found=%v err=%v", found, err)
	}
}

func TestGetResultsRest(t *testing.T) {
	for _, c := range []struct {
		param string
		row   int
		want  string
	}{
		{"", 20, "startrow=20"},
		{"performer=julia-fischer", 20, "performer=julia-fischer;startrow=20"},
		{"q=a b/c", 5, "q=a%20b%2Fc;startrow=5"},
	} {
		if got := getResultsRest(c.param, c.row); got != c.want {
			t.Errorf("getResultsRest(%q, %d) = %q, want %q", c.param, c.row, got, c.want)
		}
	}
}

// fakeBachtrack serves the search page, the deferred answer and get-results
// pages keyed by startrow.
func fakeBachtrack(t *testing.T, first string, pages map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.RequestURI()
		switch {
		case r.Method == http.MethodGet && uri == "/search-events/performer=olga-scheps":
			io.WriteString(w, deferredPage)
		case r.Method == http.MethodPost && uri == "/json/search/deferred-results":
			if err := r.ParseForm(); err != nil || r.PostForm.Get("searchfor") != "listing" || r.PostForm.Get("criteria") != "performer=olga-scheps" {
				t.Errorf("deferred-results form: %v %v", r.PostForm, err)
			}
			io.WriteString(w, first)
		case r.Method == http.MethodGet && strings.HasPrefix(uri, "/json/search/get-results/1280/listing/"):
			body, ok := pages[strings.TrimPrefix(uri, "/json/search/get-results/1280/listing/")]
			if !ok {
				http.Error(w, "no such page", http.StatusInternalServerError)
				return
			}
			io.WriteString(w, body)
		default:
			t.Errorf("unexpected request %s %s", r.Method, uri)
			http.NotFound(w, r)
		}
	}))
}

func jsonAnswer(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReadBachtrackOneAnswer(t *testing.T) {
	first := jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"html": showing(2, 2) + listing("1") + listing("2")}})
	srv := fakeBachtrack(t, first, nil)
	defer srv.Close()
	res, err := readBachtrack(srv.Client(), srv.URL, "olga-scheps")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Complete || res.Requests != 2 || len(res.Parts) != 1 {
		t.Fatalf("complete=%v requests=%d parts=%d", res.Complete, res.Requests, len(res.Parts))
	}
	if res.summary() != "COMPLETE, read to the end: 2 listing(s) (2 desktop, 2 mobile card(s)) from 2 request(s); the answer says 2 in all" {
		t.Errorf("summary: %s", res.summary())
	}
}

func TestReadBachtrackPaged(t *testing.T) {
	pager := `<a class="btk-search-more" data-startrow="2" data-param="performer=olga-scheps">See more</a>`
	first := jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"html": showing(2, 3) + listing("1") + listing("2") + pager}})
	srv := fakeBachtrack(t, first, map[string]string{
		// count as a number on one page and as a string on the next: both occur in JSON APIs.
		"performer=olga-scheps;startrow=2": jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"count": 1, "text": listing("3")}}),
		"performer=olga-scheps;startrow=3": jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"count": "0", "text": ""}}),
	})
	defer srv.Close()
	res, err := readBachtrack(srv.Client(), srv.URL, "olga-scheps")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Complete || res.Requests != 4 || len(res.Parts) != 2 {
		t.Fatalf("complete=%v requests=%d parts=%d stopped=%q", res.Complete, res.Requests, len(res.Parts), res.Stopped)
	}
	f := string(res.file())
	if i, j := strings.Index(f, `data-id='2'`), strings.Index(f, `data-id='3'`); i < 0 || j < i {
		t.Errorf("listings out of order or missing in the file:\n%s", f)
	}
	if !strings.Contains(res.summary(), "3 listing(s)") {
		t.Errorf("summary: %s", res.summary())
	}
}

func TestReadBachtrackPartlyRead(t *testing.T) {
	pager := `<a class="btk-search-more" data-startrow="2" data-param="">See more</a>`
	first := jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"html": listing("1") + pager}})
	srv := fakeBachtrack(t, first, map[string]string{}) // every get-results page fails
	defer srv.Close()
	res, err := readBachtrack(srv.Client(), srv.URL, "olga-scheps")
	if err != nil {
		t.Fatal(err)
	}
	if res.Complete || res.Stopped == "" || len(res.Parts) != 1 {
		t.Fatalf("complete=%v stopped=%q parts=%d", res.Complete, res.Stopped, len(res.Parts))
	}
	if !strings.Contains(string(res.file()), "PARTLY READ") || !strings.HasPrefix(res.summary(), "PARTLY READ") {
		t.Errorf("partial read not marked: %s", res.summary())
	}
}

// A list that ends short of the total its first answer states is not read to
// the end, even with no pager left to press.
func TestReadBachtrackShortOfTotal(t *testing.T) {
	first := jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"html": showing(2, 17) + listing("1") + listing("2")}})
	srv := fakeBachtrack(t, first, nil)
	defer srv.Close()
	res, err := readBachtrack(srv.Client(), srv.URL, "olga-scheps")
	if err != nil {
		t.Fatal(err)
	}
	if res.Complete || !strings.Contains(res.Stopped, "17 listings, and 2 were read") {
		t.Fatalf("complete=%v stopped=%q", res.Complete, res.Stopped)
	}
}

func TestReadBachtrackNotOK(t *testing.T) {
	srv := fakeBachtrack(t, `{"result":"ERROR","data":{}}`, nil)
	defer srv.Close()
	if _, err := readBachtrack(srv.Client(), srv.URL, "olga-scheps"); err == nil {
		t.Fatal("a result other than OK was taken as a list")
	}
}

func dgPage(t *testing.T, events int, next bool, cursor string) string {
	t.Helper()
	var evs []map[string]any
	for i := 0; i < events; i++ {
		evs = append(evs, map[string]any{"idRaw": i, "dateStart": "2027-01-01", "link": fmt.Sprintf("/en/x/%d", i)})
	}
	nd := map[string]any{"props": map[string]any{"pageProps": map[string]any{"componentData": map[string]any{
		"initialEvents":         evs,
		"initialEventsPageInfo": map[string]any{"hasNextPage": next, "endCursor": cursor},
		"artistId":              1191248,
	}}}}
	return `<html><script id="__NEXT_DATA__" type="application/json">` + jsonAnswer(t, nd) + `</script></html>`
}

type dgCalls struct {
	posts int
	lang  string // the language the last Load-more request asked for
}

func fakeDG(t *testing.T, page string, batches map[string]string) (*httptest.Server, *dgCalls) {
	t.Helper()
	calls := &dgCalls{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/page" || r.URL.Path == "/de/artists/maria-duenas/on-tour"):
			io.WriteString(w, page)
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			calls.posts++
			var req struct {
				Query     string                     `json:"query"`
				Variables map[string]json.RawMessage `json:"variables"`
			}
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &req); err != nil {
				t.Fatalf("graphql body: %v", err)
			}
			if r.Header.Get("Content-Type") != "application/json" || !strings.Contains(req.Query, "allLiveEvents") {
				t.Errorf("not the page's query: %s %q", r.Header.Get("Content-Type"), req.Query)
			}
			if !bytes.Equal(req.Variables["artist"], []byte("1191248")) {
				t.Errorf("artist sent as %s, want the number 1191248", req.Variables["artist"])
			}
			json.Unmarshal(req.Variables["language"], &calls.lang)
			var after string
			json.Unmarshal(req.Variables["after"], &after)
			body, ok := batches[after]
			if !ok {
				http.Error(w, "unknown cursor", http.StatusBadRequest)
				return
			}
			io.WriteString(w, body)
		default:
			http.NotFound(w, r)
		}
	})), calls
}

func batch(t *testing.T, n int, next bool, cursor string) string {
	var edges []map[string]any
	for i := 0; i < n; i++ {
		edges = append(edges, map[string]any{"node": map[string]any{"idRaw": 100 + i}})
	}
	return jsonAnswer(t, map[string]any{"data": map[string]any{"universalMusic": map[string]any{"allLiveEvents": map[string]any{
		"pageInfo": map[string]any{"hasNextPage": next, "endCursor": cursor},
		"edges":    edges,
	}}}})
}

func TestReadDG(t *testing.T) {
	srv, calls := fakeDG(t, dgPage(t, 20, true, "c1"), map[string]string{
		"c1": batch(t, 20, true, "c2"),
		"c2": batch(t, 3, false, "c3"),
	})
	defer srv.Close()
	res, err := readDG(srv.Client(), srv.URL+"/page", srv.URL+"/graphql", "en")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Complete || calls.posts != 2 || res.LoadMoreRequests != 2 || len(res.InitialEvents) != 20 || len(res.MoreEvents) != 23 {
		t.Fatalf("complete=%v posts=%d initial=%d more=%d stopped=%q", res.Complete, calls.posts, len(res.InitialEvents), len(res.MoreEvents), res.Stopped)
	}
	if res.summary() != "COMPLETE, read to the end (hasNextPage false): 20 event(s) in the page + 23 from 2 Load-more request(s), 43 in all" {
		t.Errorf("summary: %s", res.summary())
	}
	if _, err := res.file(); err != nil {
		t.Error(err)
	}
}

func TestReadDGNoMore(t *testing.T) {
	srv, calls := fakeDG(t, dgPage(t, 4, false, ""), nil)
	defer srv.Close()
	res, err := readDG(srv.Client(), srv.URL+"/page", srv.URL+"/graphql", "en")
	if err != nil || !res.Complete || calls.posts != 0 {
		t.Fatalf("err=%v complete=%v posts=%d", err, res.Complete, calls.posts)
	}
}

func TestReadDGPartlyRead(t *testing.T) {
	for name, batches := range map[string]map[string]string{
		"request fails":   {},
		"graphql errors":  {"c1": `{"errors":[{"message":"bad cursor"}]}`},
		"cursor repeated": {"c1": batch(t, 20, true, "c1")},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := fakeDG(t, dgPage(t, 20, true, "c1"), batches)
			defer srv.Close()
			res, err := readDG(srv.Client(), srv.URL+"/page", srv.URL+"/graphql", "en")
			if err != nil {
				t.Fatal(err)
			}
			if res.Complete || res.Stopped == "" || len(res.InitialEvents) != 20 {
				t.Fatalf("complete=%v stopped=%q initial=%d", res.Complete, res.Stopped, len(res.InitialEvents))
			}
		})
	}
}

func TestReadDGNoNextData(t *testing.T) {
	srv, _ := fakeDG(t, "<html>redesigned</html>", nil)
	defer srv.Close()
	if _, err := readDG(srv.Client(), srv.URL+"/page", srv.URL+"/graphql", "en"); err == nil {
		t.Fatal("a page without __NEXT_DATA__ was read as a list")
	}
}

// noNetwork fails the test on any request: refused arguments fetch nothing.
type noNetwork struct{ t *testing.T }

func (n noNetwork) RoundTrip(r *http.Request) (*http.Response, error) {
	n.t.Errorf("a refused command sent a request: %s %s", r.Method, r.URL)
	return nil, fmt.Errorf("no network in this test")
}

// The allow rule in .claude/settings.json trusts the command with any
// argument, so the arguments it refuses are part of what keeps it narrow.
// Each refusal still prints one FAILED line, the outcome word the routine
// goes by, and fetches and writes nothing.
func TestRunRefusesOtherArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"bachtrack"},
		{"bachtrack", "../../etc/passwd"},
		{"bachtrack", "Olga Scheps"},
		{"bachtrack", "olga-scheps", "extra"},
		{"dg", "https://example.com/en/artists/maria-duenas/on-tour"},
		{"dg", "https://www.deutschegrammophon.com/en/artists/maria-duenas/on-tour/../../x"},
		{"dg", "http://www.deutschegrammophon.com/en/artists/maria-duenas/on-tour"},
		{"curl", "https://bachtrack.com"},
	} {
		dir := t.TempDir()
		cfg := config{client: &http.Client{Transport: noNetwork{t}}, bachtrack: bachtrackBase, dgSite: dgSite, dgGraphQL: dgGraphQL, outDir: dir}
		var out, errb bytes.Buffer
		code := runWith(cfg, args, &out, &errb)
		if code != 1 || !strings.Contains(out.String(), ": FAILED: ") || strings.Count(out.String(), "\n") != 1 {
			t.Errorf("run(%q) = %d, stdout %q; want 1 and one FAILED line", args, code, out.String())
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("run(%q) wrote %v", args, entries)
		}
	}
	if !dgPageRE.MatchString("https://www.deutschegrammophon.com/en/artists/maria-duenas/on-tour") {
		t.Error("the routine's own label page is refused")
	}
}

// runWith is what the routine calls, so its contract is tested end to end:
// the outcome word on stdout, the exit status, and the file it writes.
func TestRunOutcomes(t *testing.T) {
	complete := jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"html": showing(1, 1) + listing("1")}})
	partial := jsonAnswer(t, map[string]any{"result": "OK", "data": map[string]any{"html": showing(1, 9) + listing("1")}})
	for _, c := range []struct {
		name, first, word string
		code              int
		file              bool
	}{
		{"complete", complete, "bachtrack olga-scheps: COMPLETE, read to the end: 1 listing(s)", 0, true},
		{"partly read", partial, "bachtrack olga-scheps: PARTLY READ (the answer says the list holds 9 listings, and 1 were read)", 2, true},
		{"failed", `{"result":"ERROR"}`, `bachtrack olga-scheps: FAILED: deferred-results answered result="ERROR"`, 1, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := fakeBachtrack(t, c.first, nil)
			defer srv.Close()
			dir := t.TempDir()
			var out, errb bytes.Buffer
			code := runWith(config{client: srv.Client(), bachtrack: srv.URL, outDir: dir}, []string{"bachtrack", "olga-scheps"}, &out, &errb)
			if code != c.code || !strings.HasPrefix(out.String(), c.word) || strings.Count(out.String(), "\n") != 1 {
				t.Fatalf("exit %d, stdout %q; want %d and one line starting %q", code, out.String(), c.code, c.word)
			}
			_, err := os.Stat(filepath.Join(dir, "bachtrack", "olga-scheps.html"))
			if (err == nil) != c.file {
				t.Errorf("file written = %v, want %v", err == nil, c.file)
			}
		})
	}
}

func TestRunDG(t *testing.T) {
	srv, calls := fakeDG(t, dgPage(t, 20, true, "c1"), map[string]string{"c1": batch(t, 2, false, "c2")})
	defer srv.Close()
	dir := t.TempDir()
	page := "https://www.deutschegrammophon.com/de/artists/maria-duenas/on-tour"
	var out, errb bytes.Buffer
	code := runWith(config{client: srv.Client(), dgSite: srv.URL, dgGraphQL: srv.URL + "/graphql", outDir: dir}, []string{"dg", page}, &out, &errb)
	if code != 0 || !strings.HasPrefix(out.String(), "dg maria-duenas: COMPLETE") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
	if calls.lang != "de" {
		t.Errorf("Load more asked for language %q, want the page's \"de\"", calls.lang)
	}
	b, err := os.ReadFile(filepath.Join(dir, "dg", "maria-duenas.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res dgResult
	if err := json.Unmarshal(b, &res); err != nil || !res.Complete || res.Page != page || len(res.MoreEvents) != 2 {
		t.Errorf("file: complete=%v page=%q more=%d err=%v", res.Complete, res.Page, len(res.MoreEvents), err)
	}
}

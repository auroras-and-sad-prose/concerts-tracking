package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const bachtrackBase = "https://bachtrack.com"

// listWidth is the browser width get-results is asked for. The page sends the
// window's width; a desktop width gets the desktop cards ROUTINE.md reads.
const listWidth = 1280

// maxListPages bounds the get-results loop. Julia Fischer's 16 listings came
// back in one answer, so fifty pages is far past any real list.
const maxListPages = 50

var slugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type part struct {
	Label string // which request produced it
	HTML  string
}

type bachtrackResult struct {
	Slug      string
	SearchFor string
	Criteria  string
	Parts     []part
	Requests  int
	Complete  bool
	Stopped   string // why reading stopped short, when !Complete
	Total     int    // listings the first answer says the list holds; 0 when it says nothing
}

// readBachtrack sends what the search page's script sends: the deferred
// request for the first answer, then get-results while the answer carries a
// btk-search-more pager (ROUTINE.md, "Secondary source"). A list that ends
// with fewer listings than the first answer's "Showing N of M listings" says
// is reported as partly read, whatever the pager did.
func readBachtrack(c *http.Client, base, slug string) (*bachtrackResult, error) {
	res, err := readBachtrackList(c, base, slug)
	if err != nil || len(res.Parts) == 0 {
		return res, err
	}
	total, ok := reportedTotal(res.Parts[0].HTML)
	if !ok {
		return res, nil
	}
	res.Total = total
	if n, _, _ := countListings(res.all()); res.Complete && n < total {
		res.Complete = false
		res.Stopped = fmt.Sprintf("the answer says the list holds %d listings, and %d were read", total, n)
	}
	return res, nil
}

func readBachtrackList(c *http.Client, base, slug string) (*bachtrackResult, error) {
	res := &bachtrackResult{Slug: slug}

	page, err := get(c, base+"/search-events/performer="+slug)
	res.Requests++
	if err != nil {
		return nil, err
	}
	sf, cr, ok := deferredAttrs(string(page))
	if !ok {
		return nil, fmt.Errorf("the search page has no btk-search-deferred element with data-search-for and data-criteria")
	}
	res.SearchFor, res.Criteria = sf, cr

	form := url.Values{"searchfor": {sf}, "criteria": {cr}}
	body, err := post(c, base+"/json/search/deferred-results", "application/x-www-form-urlencoded", form.Encode())
	res.Requests++
	if err != nil {
		return nil, err
	}
	var first struct {
		Result string `json:"result"`
		Data   struct {
			HTML string `json:"html"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &first); err != nil {
		return nil, fmt.Errorf("deferred-results: answer is not the expected JSON: %v", err)
	}
	if first.Result != "OK" {
		return nil, fmt.Errorf("deferred-results answered result=%q", first.Result)
	}
	res.Parts = append(res.Parts, part{"deferred-results searchfor=" + sf + " criteria=" + cr, first.Data.HTML})

	startrow, param, found, err := searchMore(first.Data.HTML)
	if err != nil {
		res.Stopped = err.Error()
		return res, nil
	}
	if !found {
		res.Complete = true
		return res, nil
	}
	for i := 0; ; i++ {
		if i == maxListPages {
			res.Stopped = fmt.Sprintf("still listings after %d get-results pages", maxListPages)
			return res, nil
		}
		rest := getResultsRest(param, startrow)
		body, err := get(c, base+"/json/search/get-results/"+strconv.Itoa(listWidth)+"/listing/"+rest)
		res.Requests++
		if err != nil {
			res.Stopped = err.Error()
			return res, nil
		}
		var more struct {
			Result string `json:"result"`
			Data   struct {
				Count flexInt `json:"count"`
				Text  string  `json:"text"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &more); err != nil {
			res.Stopped = fmt.Sprintf("get-results %s: answer is not the expected JSON: %v", rest, err)
			return res, nil
		}
		if more.Result != "" && more.Result != "OK" {
			res.Stopped = fmt.Sprintf("get-results %s answered result=%q", rest, more.Result)
			return res, nil
		}
		if more.Data.Count <= 0 {
			res.Complete = true
			return res, nil
		}
		res.Parts = append(res.Parts, part{"get-results " + rest + " count=" + strconv.Itoa(int(more.Data.Count)), more.Data.Text})
		startrow += int(more.Data.Count)
	}
}

// file is the list as one HTML document: every answer's cards in the order
// they came, each headed by a comment naming the request.
func (r *bachtrackResult) file() []byte {
	var b strings.Builder
	for _, p := range r.Parts {
		fmt.Fprintf(&b, "<!-- tools/fetch: %s -->\n%s\n", strings.ReplaceAll(p.Label, "--", "- -"), p.HTML)
	}
	if !r.Complete {
		fmt.Fprintf(&b, "<!-- tools/fetch: PARTLY READ: %s -->\n", strings.ReplaceAll(r.Stopped, "--", "- -"))
	}
	return []byte(b.String())
}

func (r *bachtrackResult) all() string {
	var all strings.Builder
	for _, p := range r.Parts {
		all.WriteString(p.HTML)
	}
	return all.String()
}

func (r *bachtrackResult) summary() string {
	n, desk, mob := countListings(r.all())
	s := fmt.Sprintf("%d listing(s) (%d desktop, %d mobile card(s)) from %d request(s)", n, desk, mob, r.Requests)
	if r.Total > 0 {
		s += fmt.Sprintf("; the answer says %d in all", r.Total)
	}
	if r.Complete {
		return "COMPLETE, read to the end: " + s
	}
	return "PARTLY READ (" + r.Stopped + "): " + s
}

// getResultsRest is the path tail the pager's script builds:
// "<param>;startrow=<n>", or "startrow=<n>" when data-param is empty.
func getResultsRest(param string, startrow int) string {
	rest := "startrow=" + strconv.Itoa(startrow)
	if param != "" {
		rest = param + ";" + rest
	}
	return escapePath(rest)
}

// escapePath percent-encodes what cannot stand in a URL path, leaving the
// characters the pager's own parameters use (= ; , & and the like) as typed.
// url.PathEscape would encode the ';' that separates them.
func escapePath(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case 'a' <= ch && ch <= 'z', 'A' <= ch && ch <= 'Z', '0' <= ch && ch <= '9',
			strings.IndexByte("-._~=;,&+:@!$'()*%", ch) >= 0:
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

func deferredAttrs(page string) (searchFor, criteria string, ok bool) {
	for _, a := range tagsWithClass(page, "btk-search-deferred") {
		sf, ok1 := a["data-search-for"]
		cr, ok2 := a["data-criteria"]
		if ok1 && ok2 && sf != "" && cr != "" {
			return sf, cr, true
		}
	}
	return "", "", false
}

func searchMore(list string) (startrow int, param string, found bool, err error) {
	tags := tagsWithClass(list, "btk-search-more")
	if len(tags) == 0 {
		return 0, "", false, nil
	}
	a := tags[0]
	n, err := strconv.Atoi(strings.TrimSpace(a["data-startrow"]))
	if err != nil {
		return 0, "", true, fmt.Errorf("btk-search-more pager has no usable data-startrow (%q)", a["data-startrow"])
	}
	return n, a["data-param"], true, nil
}

// countListings counts the listings (distinct data-id on the wrapper each
// card sits in, which carries data-type; the wish-list button repeats the id
// without it), and the desktop (listing-shortform) and mobile (listing-ms)
// cards separately, so a listing missing one of its two cards shows up in the
// summary.
func countListings(list string) (listings, desktop, mobile int) {
	ids := map[string]bool{}
	for _, a := range allTags(list) {
		if _, wrapper := a["data-type"]; wrapper && a["data-id"] != "" {
			ids[a["data-id"]] = true
		}
	}
	desktop = len(tagsWithClass(list, "listing-shortform"))
	mobile = len(tagsWithClass(list, "listing-ms"))
	return len(ids), desktop, mobile
}

var totalRE = regexp.MustCompile(`class=['"]btk-search-result-total['"][^>]*>\s*(\d+)\s*<`)

// reportedTotal reads M from the answer's "Showing N of M listings" line.
func reportedTotal(list string) (int, bool) {
	m := totalRE.FindStringSubmatch(list)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

var (
	// tagRE matches a start tag, stepping over quoted attribute values so
	// that a '>' inside one (a data-label reading "Bach > Brahms") does not
	// end the tag early.
	tagRE  = regexp.MustCompile(`<[a-zA-Z][a-zA-Z0-9-]*(?:\s(?:[^"'<>]|"[^"]*"|'[^']*')*)?>`)
	attrRE = regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))`)
)

// tagsWithClass returns the attributes of every start tag whose class list
// holds class as a whole token ("listing-ms" does not match "listing-ms-city").
func tagsWithClass(doc, class string) []map[string]string {
	var out []map[string]string
	for _, tag := range tagRE.FindAllString(doc, -1) {
		if !strings.Contains(tag, class) {
			continue
		}
		attrs := tagAttrs(tag)
		for _, tok := range strings.Fields(attrs["class"]) {
			if tok == class {
				out = append(out, attrs)
				break
			}
		}
	}
	return out
}

func allTags(doc string) []map[string]string {
	var out []map[string]string
	for _, tag := range tagRE.FindAllString(doc, -1) {
		out = append(out, tagAttrs(tag))
	}
	return out
}

// tagAttrs reads a start tag's attributes. The first occurrence of a name
// wins, as in a browser.
func tagAttrs(tag string) map[string]string {
	attrs := map[string]string{}
	for _, m := range attrRE.FindAllStringSubmatch(tag, -1) {
		name := strings.ToLower(m[1])
		if _, seen := attrs[name]; !seen {
			attrs[name] = html.UnescapeString(m[2] + m[3] + m[4])
		}
	}
	return attrs
}

// flexInt reads a count sent either as a JSON number or as a numeric string.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("count %s is not a number", b)
	}
	*f = flexInt(n)
	return nil
}

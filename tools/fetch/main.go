// Command fetch makes the two script-driven list reads the concert-watch
// routine needs (ROUTINE.md, "Secondary source" and "Tertiary source"), as
// one checked-in command rather than hand-typed curl lines. Run it from the
// repo root:
//
//	go run ./tools/fetch bachtrack <performer-slug>
//	go run ./tools/fetch dg <deutschegrammophon.com on-tour page URL>
//
// Each writes what it read under .cache/fetch/ (gitignored) and prints one
// line to stdout naming the outcome: COMPLETE (the list was read to its end),
// PARTLY READ (the file holds what was read, and the line says why it
// stopped) or FAILED (nothing usable was read, and no file is written). That
// word is the contract. The exit status is 0 only for COMPLETE, but don't
// read more into it: the program exits 2 for PARTLY READ and 1 for FAILED,
// and `go run` reports both as 1.
//
// The project's .claude/settings.json allows exactly these two invocations,
// so a scheduled run can send the requests without a permission prompt. That
// is why the command is this narrow: it takes nothing but a performer slug or
// an on-tour page on deutschegrammophon.com, talks only to bachtrack.com and
// to the Universal Music API that page's "Load more" button calls, and writes
// only under .cache/fetch/.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = `usage:
  go run ./tools/fetch bachtrack <performer-slug>
  go run ./tools/fetch dg https://www.deutschegrammophon.com/<lang>/artists/<slug>/on-tour`

// outDir is where every read lands, relative to the repo root.
const outDir = ".cache/fetch"

// dgSite is the only site an on-tour page may come from.
const dgSite = "https://www.deutschegrammophon.com"

// config is where run sends its requests and writes its files: the live
// sites in main, local fakes in the tests.
type config struct {
	client    *http.Client
	bachtrack string // base URL of bachtrack.com
	dgSite    string // base URL the on-tour page is fetched from
	dgGraphQL string
	outDir    string
}

// userAgent names the tool honestly: CLAUDE.md rules out faked browser
// headers, and neither endpoint has asked for one.
const userAgent = "concerts-tracking-fetch/1 (scheduled concert-watch routine)"

// maxBody caps any single answer; the longest list seen is well under 1 MB.
const maxBody = 32 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWith(config{
		client:    &http.Client{Timeout: 60 * time.Second},
		bachtrack: bachtrackBase,
		dgSite:    dgSite,
		dgGraphQL: dgGraphQL,
		outDir:    outDir,
	}, args, stdout, stderr)
}

func runWith(cfg config, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, usage)
		return 1
	}
	c := cfg.client
	switch args[0] {
	case "bachtrack":
		slug := args[1]
		if !slugRE.MatchString(slug) {
			fmt.Fprintf(stderr, "error: %q is not a Bachtrack performer slug (lowercase letters, digits and hyphens)\n", slug)
			return 1
		}
		res, err := readBachtrack(c, cfg.bachtrack, slug)
		if err != nil {
			fmt.Fprintf(stdout, "bachtrack %s: FAILED: %v\n", slug, err)
			return 1
		}
		path := filepath.Join(cfg.outDir, "bachtrack", slug+".html")
		if err := writeFile(path, res.file()); err != nil {
			fmt.Fprintf(stdout, "bachtrack %s: FAILED: %v\n", slug, err)
			return 1
		}
		fmt.Fprintf(stdout, "bachtrack %s: %s -> %s\n", slug, res.summary(), path)
		return exitFor(res.Complete)
	case "dg":
		m := dgPageRE.FindStringSubmatch(args[1])
		if m == nil {
			fmt.Fprintf(stderr, "error: %q is not a deutschegrammophon.com artist on-tour page\n", args[1])
			return 1
		}
		res, err := readDG(c, cfg.dgSite+strings.TrimPrefix(args[1], dgSite), cfg.dgGraphQL, m[1])
		if err != nil {
			fmt.Fprintf(stdout, "dg %s: FAILED: %v\n", m[2], err)
			return 1
		}
		res.Page = args[1]
		b, err := res.file()
		if err == nil {
			path := filepath.Join(cfg.outDir, "dg", m[2]+".json")
			if err = writeFile(path, b); err == nil {
				fmt.Fprintf(stdout, "dg %s: %s -> %s\n", m[2], res.summary(), path)
				return exitFor(res.Complete)
			}
		}
		fmt.Fprintf(stdout, "dg %s: FAILED: %v\n", m[2], err)
		return 1
	default:
		fmt.Fprintln(stderr, usage)
		return 1
	}
}

// exitFor is 0 for a complete read and 2 for a partial one. Under `go run`
// the 2 arrives as 1, so callers read the outcome word, not this.
func exitFor(complete bool) int {
	if complete {
		return 0
	}
	return 2
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func get(c *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return do(c, req)
}

func post(c *http.Client, u, contentType string, body string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return do(c, req)
}

func do(c *http.Client, req *http.Request) ([]byte, error) {
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: %v", req.Method, req.URL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: HTTP %d", req.Method, req.URL, resp.StatusCode)
	}
	if len(b) > maxBody {
		return nil, errors.New(req.Method + " " + req.URL.String() + ": answer larger than 32 MB")
	}
	return b, nil
}

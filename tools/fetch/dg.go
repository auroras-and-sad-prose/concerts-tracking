package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

// dgGraphQL is the API the on-tour page's "Load more" button posts to.
const dgGraphQL = "https://graphql.universal-music.de"

// maxLoadMore bounds the Load-more loop: at 20 events a batch and a list
// capped at one year ahead, thirty batches is far past any real tour.
const maxLoadMore = 30

var dgPageRE = regexp.MustCompile(`^https://www\.deutschegrammophon\.com/([a-z]{2})/artists/([a-z0-9]+(?:-[a-z0-9]+)*)/on-tour$`)

var nextDataRE = regexp.MustCompile(`(?s)<script[^>]*\bid="__NEXT_DATA__"[^>]*>(.*?)</script>`)

// dgQuery is the page's own query, copied from its script bundle
// (ROUTINE.md, "Tertiary source").
const dgQuery = `query GetAllLiveEvents($channel: Int $language: String $first: Int
  $sortBy: SortMethod $sortOrder: SortOrder $dateFrom: String $dateTo: String
  $location: String $onlyGsaCountries: Boolean $umgClassicsOnly: Boolean
  $artist: Int $after: String) {
  universalMusic(channel: $channel, language: $language) {
    allLiveEvents(after: $after first: $first sortBy: $sortBy
      sortOrder: $sortOrder dateFrom: $dateFrom dateTo: $dateTo
      locationName: $location artist: $artist
      onlyGsaCountries: $onlyGsaCountries umgClassicsOnly: $umgClassicsOnly) {
      pageInfo { hasNextPage endCursor }
      edges { node { parentId idRaw headline dateStart timeStart repertoire
        locationPartners { name city country } } }
    }
  }
}`

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type dgResult struct {
	Page     string      `json:"page"`
	ArtistID json.Number `json:"artistId"`
	Complete bool        `json:"complete"`
	Stopped  string      `json:"stopped,omitempty"`
	// LoadMoreRequests counts the GraphQL batches sent after the page itself.
	LoadMoreRequests int `json:"loadMoreRequests"`
	// InitialEvents are the events in the page's HTML; they carry a "link".
	InitialEvents []json.RawMessage `json:"initialEvents"`
	// MoreEvents came from Load-more batches and carry no link (ROUTINE.md:
	// such a row's detail_url stays null from this source).
	MoreEvents []json.RawMessage `json:"moreEvents"`
	PageInfo   pageInfo          `json:"lastPageInfo"`
}

// readDG reads the events in the page's __NEXT_DATA__, then presses "Load
// more" the way the page does until hasNextPage is false.
func readDG(c *http.Client, pageURL, graphqlURL string) (*dgResult, error) {
	page, err := get(c, pageURL)
	if err != nil {
		return nil, err
	}
	m := nextDataRE.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("the page has no __NEXT_DATA__ script")
	}
	var nd struct {
		Props struct {
			PageProps struct {
				ComponentData struct {
					InitialEvents []json.RawMessage `json:"initialEvents"`
					PageInfo      *pageInfo         `json:"initialEventsPageInfo"`
					ArtistID      json.Number       `json:"artistId"`
				} `json:"componentData"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(m[1], &nd); err != nil {
		return nil, fmt.Errorf("__NEXT_DATA__ does not parse: %v", err)
	}
	cd := nd.Props.PageProps.ComponentData
	if cd.InitialEvents == nil || cd.PageInfo == nil {
		return nil, fmt.Errorf("__NEXT_DATA__ has no componentData.initialEvents / initialEventsPageInfo")
	}
	res := &dgResult{
		Page:          pageURL,
		ArtistID:      cd.ArtistID,
		InitialEvents: cd.InitialEvents,
		MoreEvents:    []json.RawMessage{},
		PageInfo:      *cd.PageInfo,
	}
	if !res.PageInfo.HasNextPage {
		res.Complete = true
		return res, nil
	}
	if _, err := res.ArtistID.Int64(); err != nil {
		res.Stopped = fmt.Sprintf("componentData.artistId %q is not a number, so Load more cannot be sent", res.ArtistID)
		return res, nil
	}

	for res.PageInfo.HasNextPage {
		if res.LoadMoreRequests == maxLoadMore {
			res.Stopped = fmt.Sprintf("hasNextPage still true after %d Load-more requests", maxLoadMore)
			return res, nil
		}
		body, err := json.Marshal(map[string]any{
			"query": dgQuery,
			"variables": map[string]any{
				"channel":          54,
				"language":         "en",
				"first":            20,
				"sortBy":           "DATE",
				"sortOrder":        "ASCENDING",
				"onlyGsaCountries": false,
				"umgClassicsOnly":  true,
				"dateTo":           "now +1 year",
				"artist":           res.ArtistID,
				"after":            res.PageInfo.EndCursor,
			},
		})
		if err != nil {
			return nil, err
		}
		answer, err := post(c, graphqlURL, "application/json", string(body))
		res.LoadMoreRequests++
		if err != nil {
			res.Stopped = err.Error()
			return res, nil
		}
		var g struct {
			Data struct {
				UniversalMusic struct {
					AllLiveEvents *struct {
						PageInfo pageInfo `json:"pageInfo"`
						Edges    []struct {
							Node json.RawMessage `json:"node"`
						} `json:"edges"`
					} `json:"allLiveEvents"`
				} `json:"universalMusic"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(answer, &g); err != nil {
			res.Stopped = fmt.Sprintf("Load-more answer is not the expected JSON: %v", err)
			return res, nil
		}
		if len(g.Errors) > 0 {
			res.Stopped = "Load-more answer carries GraphQL errors: " + g.Errors[0].Message
			return res, nil
		}
		ev := g.Data.UniversalMusic.AllLiveEvents
		if ev == nil {
			res.Stopped = "Load-more answer has no data.universalMusic.allLiveEvents"
			return res, nil
		}
		for _, e := range ev.Edges {
			res.MoreEvents = append(res.MoreEvents, e.Node)
		}
		if ev.PageInfo.HasNextPage && ev.PageInfo.EndCursor == res.PageInfo.EndCursor {
			res.Stopped = "Load more returned the same endCursor again"
			return res, nil
		}
		res.PageInfo = ev.PageInfo
	}
	res.Complete = true
	return res, nil
}

func (r *dgResult) file() ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (r *dgResult) summary() string {
	s := fmt.Sprintf("%d event(s) in the page + %d from %d Load-more request(s), %d in all",
		len(r.InitialEvents), len(r.MoreEvents), r.LoadMoreRequests, len(r.InitialEvents)+len(r.MoreEvents))
	if r.Complete {
		return "read to the end (hasNextPage false), " + s
	}
	return "PARTLY READ (" + r.Stopped + "), " + s
}

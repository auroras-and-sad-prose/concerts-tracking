// Command gen regenerates tools/stations/de.csv from the trainline-eu/stations
// dataset: every German station that has a Deutsche Bahn station number, as
// "name;eva", sorted by name. Run it from the repo root:
//
//	curl -sSLo /tmp/stations.csv https://raw.githubusercontent.com/trainline-eu/stations/master/stations.csv
//	go run ./tools/stations/gen -in /tmp/stations.csv -out tools/stations/de.csv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/auroras-and-sad-prose/concerts-tracking/tools/stations"
)

func main() {
	in := flag.String("in", "", "path to trainline-eu stations.csv")
	out := flag.String("out", "tools/stations/de.csv", "path to write")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return err
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, need := range []string{"name", "country", "db_id"} {
		if _, ok := col[need]; !ok {
			return fmt.Errorf("%s: no %q column", in, need)
		}
	}

	var rows []stations.Station
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, eva := strings.TrimSpace(rec[col["name"]]), rec[col["db_id"]]
		if rec[col["country"]] != "DE" || eva == "" {
			continue
		}
		if name == "" || strings.ContainsAny(name, ";\n") || !stations.ValidEVA(eva) {
			return fmt.Errorf("unusable row: %q;%q", name, eva)
		}
		rows = append(rows, stations.Station{Name: name, EVA: eva})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].EVA < rows[j].EVA
	})

	var b strings.Builder
	b.WriteString("name;eva\n")
	for _, s := range rows {
		b.WriteString(s.Name + ";" + s.EVA + "\n")
	}
	return os.WriteFile(out, []byte(b.String()), 0o644)
}

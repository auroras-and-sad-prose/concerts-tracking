# German station list (`de.csv`)

`de.csv` lists every German station in the
[trainline-eu/stations](https://github.com/trainline-eu/stations) dataset that
has a Deutsche Bahn station number: one `name;eva` row per station, sorted by
name. The concert-watch routine looks a city's station up here
(`go run ./tools/validate -find-station "<German name>"`) and copies the result
into `travel.json`. CI checks every station in `travel.json` against this file.
The page needs the number because bahn.de ignores a destination given by name
alone.

## Licence

`de.csv` is a derivative of trainline-eu/stations, which is made available
under the [Open Database License (ODbL) v1.0](https://opendatacommons.org/licenses/odbl/1-0/).
Accordingly, `de.csv` is itself made available under the ODbL. It contains
information from trainline-eu/stations; the rest of this repository is not
covered by that licence.

## Regenerating

```sh
curl -sSLo /tmp/stations.csv https://raw.githubusercontent.com/trainline-eu/stations/master/stations.csv
go run ./tools/stations/gen -in /tmp/stations.csv -out tools/stations/de.csv
go test ./tools/...
```

Regenerating is a reviewed change like any other edit under `tools/`; the
routine never does it. A station renamed or renumbered upstream will fail CI
for any `travel.json` entry that still names the old one. Fix that entry by
looking the city up again.

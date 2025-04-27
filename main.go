package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/biter777/countries"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/uber/h3-go/v4"
)

const fileNameTemplate = "uberh3_%s_res_%d.csv"

func main() {
	polygonsByCountries, err := getPolygonsByCountries()
	if err != nil {
		log.Fatal(err)
	}
	resolutions := []int{4}
	generateUberH3CellsForCountries(polygonsByCountries, resolutions)
}

// generateUberH3CellsForLand generates one file for each country, and it also generates one <land>
// file that contains all cells from all countries.
func generateUberH3CellsForCountries(polygonsByCountries map[string][]h3.GeoPolygon, resolutions []int) {
	for _, res := range resolutions {
		landCells := make(map[h3.Cell]bool)
		for country, polygons := range polygonsByCountries {
			countryCells := make(map[h3.Cell]bool)
			for _, polygon := range polygons {
				cells, _ := h3.PolygonToCells(polygon, res)
				for _, c := range cells {
					countryCells[c] = true
					landCells[c] = true
				}
			}
			writeCellsToFile(fmt.Sprintf(fileNameTemplate, strings.ToLower(country), res), slices.Collect(maps.Keys(countryCells)))
			slog.Info("Done writing cells",
				"name", country,
				"res", res,
				"count ", len(countryCells))
		}
		writeCellsToFile(fmt.Sprintf(fileNameTemplate, "land", res), slices.Collect(maps.Keys(landCells)))
		slog.Info("Done writing cells",
			"name", "land",
			"res", res,
			"count ", len(landCells))
	}
}

func getPolygonsByCountries() (map[string][]h3.GeoPolygon, error) {
	resp, err := http.Get("https://raw.githubusercontent.com/nvkelso/natural-earth-vector/refs/heads/master/geojson/ne_110m_admin_0_countries.geojson")
	if err != nil {
		return nil, err
	}
	countriesGeo, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var fc geojson.FeatureCollection
	err = json.Unmarshal(countriesGeo, &fc)
	if err != nil {
		return nil, err
	}

	polygonsByCountries := make(map[string][]h3.GeoPolygon)
	for _, f := range fc.Features {
		countryCode := "UNKNOWN"
		countryISOAlpha3, ok := f.Properties["ISO_A3"].(string)
		if ok && countries.ByName(countryISOAlpha3) != countries.Unknown {
			countryCode = countryISOAlpha3
		} else {
			countryISOAlpha3, ok := f.Properties["ISO_A3_EH"].(string)
			if ok && countries.ByName(countryISOAlpha3) != countries.Unknown {
				countryCode = countryISOAlpha3
			}
		}

		var polygons []h3.GeoPolygon
		addPolygon := func(polygon orb.Polygon) {
			var outer []h3.LatLng
			for _, ring := range polygon {
				for _, pt := range ring {
					outer = append(outer, h3.LatLng{
						Lat: pt[1],
						Lng: pt[0],
					})
				}
				break
			}
			polygons = append(polygons, h3.GeoPolygon{
				GeoLoop: outer,
			})
		}

		if polygon, ok := f.Geometry.(orb.Polygon); ok {
			addPolygon(polygon)
		} else if multiPolygon, ok := f.Geometry.(orb.MultiPolygon); ok {
			for _, polygon := range multiPolygon {
				addPolygon(polygon)
			}
		} else {
			log.Fatal("not polygon")
		}
		polygonsByCountries[countryCode] = append(polygonsByCountries[countryCode], polygons...)
	}
	return polygonsByCountries, nil
}

func writeCellsToFile(path string, cells []h3.Cell) {
	if _, err := os.Stat("resources"); os.IsNotExist(err) {
		os.Mkdir("resources", 0755)
	}
	path = "resources/" + path

	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"id", "lat", "lng"})
	for _, c := range cells {
		ll, _ := c.LatLng()
		record := []string{
			strconv.FormatUint(uint64(c), 10),
			strconv.FormatFloat(ll.Lat, 'f', 6, 64),
			strconv.FormatFloat(ll.Lng, 'f', 6, 64),
		}
		writer.Write(record)
	}
}

// generateUberH3CellsForEarth is not very useful since it contains cells in ocean
func generateUberH3CellsForEarth() {
	for _, res := range []int{4} {
		cells := getAllCellsAtAResolution(res)
		writeCellsToFile(fmt.Sprintf(fileNameTemplate, "earth", res), cells)
		slog.Info("Done writing", "res", res, "count", len(cells))
	}
}

func getAllCellsAtAResolution(res int) []h3.Cell {
	var all []h3.Cell
	res0, _ := h3.Res0Cells()
	for _, r0 := range res0 {
		children, _ := r0.Children(res)
		all = append(all, children...)
	}
	return all
}

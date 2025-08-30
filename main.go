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

const fileNameTemplate = "h3_res_%d_%s.csv"

var resolutions = []int{4}

func main() {
	polygonsByCountries, err := getCountryPolygons()
	if err != nil {
		log.Fatal(err)
	}

	for _, res := range resolutions {
		for country, polygons := range polygonsByCountries {
			countryCells := make(map[h3.Cell]bool)
			for _, polygon := range polygons {
				cells, _ := h3.PolygonToCells(polygon, res)
				for _, c := range cells {
					countryCells[c] = true
				}
			}
			writeCellsToFile(fmt.Sprintf(fileNameTemplate, res, strings.ToLower(country)), slices.Collect(maps.Keys(countryCells)))
			slog.Info("Done writing cells", "name", country, "res", res, "count", len(countryCells))
		}
	}
}

// Key is country code, value is list of polygons
func getCountryPolygons() (map[string][]h3.GeoPolygon, error) {
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

	countryPolygons := make(map[string][]h3.GeoPolygon)
	for _, f := range fc.Features {
		countryCode := "UNKNOWN"
		countryISOA3, ok := f.Properties["ISO_A3"].(string)
		if ok && countries.ByName(countryISOA3) != countries.Unknown {
			countryCode = countryISOA3
		} else {
			countryISOA3, ok := f.Properties["ISO_A3_EH"].(string)
			if ok && countries.ByName(countryISOA3) != countries.Unknown {
				countryCode = countryISOA3
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
			}
			polygons = append(polygons, h3.GeoPolygon{GeoLoop: outer})
		}

		if polygon, ok := f.Geometry.(orb.Polygon); ok {
			addPolygon(polygon)
		} else if multiPolygon, ok := f.Geometry.(orb.MultiPolygon); ok {
			for _, polygon := range multiPolygon {
				addPolygon(polygon)
			}
		} else {
			panic("unknown data format from natural earth")
		}
		countryPolygons[countryCode] = append(countryPolygons[countryCode], polygons...)
	}
	return countryPolygons, nil
}

func writeCellsToFile(path string, cells []h3.Cell) {
	if _, err := os.Stat("data"); os.IsNotExist(err) {
		os.Mkdir("data", 0755)
	}
	path = "data/" + path

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

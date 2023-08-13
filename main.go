package main

import (
	// exiftool "github.com/barasher/go-exiftool"
	"encoding/json"
	"os"
	"time"

	goflag "flag"

	flag "github.com/spf13/pflag"

	"github.com/google/btree"
	"github.com/sirupsen/logrus"
)

type LocationFile struct {
	Locations []Location `json:"locations"`
}

type Location struct {
	LatitudeE7  int       `json:"latitudeE7"`
	LongitudeE7 int       `json:"longitudeE7"`
	Timestamp   time.Time `json:"timestamp"`
}

func locationLessFunc(a, b Location) bool {
	return a.Timestamp.Before(b.Timestamp)
}

// reads locations from a Records.json google takeout file and returns a btree with all the locations
func readLocations(locationFile string) (*btree.BTreeG[Location], error) {
	bytes, err := os.ReadFile(locationFile)
	if err != nil {
		return nil, err
	}

	var locations LocationFile
	err = json.Unmarshal(bytes, &locations)
	if err != nil {
		return nil, err
	}

	btree := btree.NewG[Location](2, locationLessFunc)
	for _, v := range locations.Locations {
		btree.ReplaceOrInsert(v)
	}

	return btree, nil
}

func getUnsignedDateDifference(a, b time.Time) time.Duration {
	if a.Before(b) {
		return b.Sub(a)
	}
	return a.Sub(b)
}

var locationFile = flag.StringP("locationFile", "f", "", "path to the location file")
var dateToFind = flag.StringP("dateToFind", "d", "", "date to find in the location file")
var tolerance = flag.DurationP("tolerance", "t", 1*time.Hour, "tolerance for the date to find (e.g. 1h, 30m, 1h30m, 1h30m30s, etc.)")

func main() {
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	flag.Parse()

	// parse dateToFind to a time object in RFC3339 format
	dateToFindTime, err := time.Parse(time.RFC3339, *dateToFind)
	if err != nil {
		panic(err)
	}

	locations, err := readLocations(*locationFile)

	if err != nil {
		panic(err)
	}

	logrus.Infof("Read %v locations", locations.Len())

	// find closest match to datetofindtime in locations btree
	var closestMatch Location
	var closestMatchDifference *time.Duration
	locations.Ascend(func(l Location) bool {
		currentDifference := getUnsignedDateDifference(l.Timestamp, dateToFindTime)
		if closestMatchDifference != nil && currentDifference > *closestMatchDifference {
			closestMatch = l
			closestMatchDifference = &currentDifference

			return false
		}

		closestMatch = l
		closestMatchDifference = &currentDifference

		return true
	})

	if *closestMatchDifference < *tolerance {
		logrus.Infof("No location found within the defined tolerance.")
		os.Exit(1)
	}

	logrus.Infof("closest match: %v, %v", closestMatch.LatitudeE7, closestMatch.LongitudeE7)
	logrus.Infof("closest match divided: %v, %v", float32(closestMatch.LatitudeE7)/10000000, float32(closestMatch.LongitudeE7)/10000000)
}

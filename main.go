package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	goflag "flag"

	flag "github.com/spf13/pflag"

	exiftool "github.com/barasher/go-exiftool"
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

func findLocationFromDate(locations *btree.BTreeG[Location], dateToFindTime time.Time) *Location {
	var closestMatch *Location
	closestMatchDifference := 999999 * time.Hour
	locations.AscendRange(Location{Timestamp: dateToFindTime.Add(-*tolerance)}, Location{Timestamp: dateToFindTime.Add(*tolerance)}, (func(l Location) bool {
		currentDifference := getUnsignedDateDifference(l.Timestamp, dateToFindTime)

		// Stop right away if the exact date is found
		if l.Timestamp == dateToFindTime {
			closestMatch = &l
			return false
		}

		if currentDifference < closestMatchDifference {
			closestMatch = &l
			closestMatchDifference = currentDifference
		}

		return true
	}))

	if closestMatch == nil || closestMatchDifference > *tolerance {
		logrus.Tracef("No location found within the defined tolerance.")
		return nil
	}
	return closestMatch
}

var locationFile = flag.StringP("locationFile", "f", "", "path to the location file")
var photosDirectory = flag.StringP("photos-directory", "d", "", "path to the photos directory")
var tolerance = flag.DurationP("tolerance", "t", 1*time.Hour, "tolerance for the date to find (e.g. 1h, 30m, 1h30m, 1h30m30s, etc.)")
var skipBackup = flag.Bool("skip-backup", false, "skip backup of the photos before modifying them")
var skipPrompt = flag.BoolP("skip-promt", "y", false, "skip the prompt before modifying the photos")
var dryRun = flag.BoolP("dry-run", "n", false, "skips all write operations")
var verbose = flag.BoolP("verbose", "v", false, "verbose output")

func main() {
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}

	locations, err := readLocations(*locationFile)

	if err != nil {
		panic(err)
	}

	logrus.Infof("Read %v locations", locations.Len())
	unsupportedExtensions := map[string]int{}
	filesToProcess := []string{}

	filepath.WalkDir(*photosDirectory, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsDir() {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(d.Name()))
		if extension != ".jpg" && extension != ".jpeg" {
			unsupportedExtensions[extension]++
			return nil
		}

		filesToProcess = append(filesToProcess, path)

		return nil
	})

	logrus.Infof("Found:")
	logrus.Infof("\tUnsupported extensions: %v", unsupportedExtensions)
	logrus.Infof("\tFiles with supported extensions: %v", len(filesToProcess))

	logrus.Infof("Starting the exif read operation and backups")
	var et *exiftool.Exiftool
	if *skipBackup == false {
		et, err = exiftool.NewExiftool(exiftool.BackupOriginal())
		if err != nil {
			logrus.Fatalf("Error when intializing exiftool: %v\n", err)
		}
		defer et.Close()
	} else {
		et, err = exiftool.NewExiftool()
		if err != nil {
			logrus.Fatalf("Error when intializing exiftool: %v\n", err)
		}
		defer et.Close()
	}

	noLocationFoundCounter, noDateTimeCounter, gpsMetadataAlreadySetCounter := 0, 0, 0

	files := et.ExtractMetadata(filesToProcess...)
	filesPreparedToWrite := []exiftool.FileMetadata{}
	for _, fileinfo := range files {
		if fileinfo.Err != nil {
			logrus.Fatalf("Error when extracting metadata: %v\n", fileinfo.Err)
		}

		if fileinfo.Fields["GPSLatitude"] != nil || fileinfo.Fields["GPSLongitude"] != nil {
			logrus.Debugf("Skipping file %v because it already has GPS metadata", fileinfo.File)
			gpsMetadataAlreadySetCounter++
			continue
		}

		dateTimeOriginal, err := fileinfo.GetString("DateTimeOriginal")
		if err != nil {
			logrus.Warnf("Skipping file %v because we couldn't determine the time the photo was taken: %v", fileinfo.File, err)
			noDateTimeCounter++
			continue
		}
		dtParse, err := time.Parse("2006:01:02 15:04:05", dateTimeOriginal)
		if err != nil {
			logrus.Warnf("Skipping file %v because we couldn't parse the time the photo was taken: %v", fileinfo.File, err)
			noDateTimeCounter++
			continue
		}

		location := findLocationFromDate(locations, dtParse)
		if location == nil {
			logrus.Warnf("No location found within the defined tolerance for file %v", fileinfo.File)
			noLocationFoundCounter++
			continue
		}
		latitude := float32(location.LatitudeE7) / 10000000
		longitude := float32(location.LongitudeE7) / 10000000
		logrus.Debugf("Found location for file %v: %v, %v", fileinfo.File, latitude, longitude)

		fileinfo.Fields["GPSLatitude"] = latitude
		fileinfo.Fields["GPSLatitudeRef"] = latitude
		fileinfo.Fields["GPSLongitude"] = longitude
		fileinfo.Fields["GPSLongitudeRef"] = longitude

		filesPreparedToWrite = append(filesPreparedToWrite, fileinfo)
	}

	logrus.Infof("Finished the exif read operation")

	logrus.Infof("Summary:")
	logrus.Infof("\tFiles supported: %v", len(filesToProcess))
	logrus.Infof("\tFiles with no location found: %v", noLocationFoundCounter)
	logrus.Infof("\tFiles with no date time found: %v", noDateTimeCounter)
	logrus.Infof("\tFiles with GPS metadata already set: %v", gpsMetadataAlreadySetCounter)

	if *skipPrompt == false {
		logrus.Infof("%v files will be modified. Do you wish to proceed? (Yes/No)", len(filesPreparedToWrite))

		if requestConfirmation() == false {
			logrus.Infof("Aborting.")
			os.Exit(0)
		}
	} else {
		logrus.Infof("Skipping confirmation prompt.")
	}

	logrus.Infof("Starting the exif rewrite operation")

	errorWriteCounter := 0
	successfulWriteCounter := 0

	if *dryRun == false {
		et.WriteMetadata(filesPreparedToWrite)
		for _, v := range filesPreparedToWrite {
			if v.Err != nil {
				logrus.Warnf("Error when writing metadata for file %v: %v", v.File, v.Err)
				errorWriteCounter++
			} else {
				successfulWriteCounter++
			}
		}
	}

	logrus.Infof("Finished the exif rewrite operation")
	logrus.Infof("Summary:")
	logrus.Infof("\tFiles supported: %v", len(filesToProcess))
	logrus.Infof("\tSucessfully processed files: %v", successfulWriteCounter)
	logrus.Infof("\tFiles with no location found: %v", noLocationFoundCounter)
	logrus.Infof("\tFiles with no date time found: %v", noDateTimeCounter)
	logrus.Infof("\tFiles with GPS metadata already set: %v", gpsMetadataAlreadySetCounter)
	logrus.Infof("\tFiles with write failure: %v", errorWriteCounter)
}

func requestConfirmation() bool {
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		logrus.Fatal(err)
	}

	response = strings.ToLower(strings.TrimSpace(response))

	if response == "y" || response == "yes" {
		return true
	} else if response == "n" || response == "no" {
		return false
	}
	return false
}

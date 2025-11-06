package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	exiftool "github.com/barasher/go-exiftool"
)

// E2E tests using sample_data

func TestE2E_ProcessSampleData(t *testing.T) {
	// Create a temp directory for test files
	tmpDir := t.TempDir()
	
	// Copy sample images to temp directory
	sampleDir := "sample_data/Google Photos/Untitled"
	files, err := os.ReadDir(sampleDir)
	if err != nil {
		t.Fatalf("Failed to read sample directory: %v", err)
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		srcPath := filepath.Join(sampleDir, file.Name())
		dstPath := filepath.Join(tmpDir, file.Name())
		
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", srcPath, err)
		}
		
		err = os.WriteFile(dstPath, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", dstPath, err)
		}
	}
	
	// Test reading locations
	locationFile := "sample_data/Location History/Records.json"
	locations, err := readLocations(locationFile)
	if err != nil {
		t.Fatalf("Failed to read locations: %v", err)
	}
	
	if locations.Len() != 5 {
		t.Errorf("Expected 5 locations, got %d", locations.Len())
	}
	
	// Setup exiftool
	et, err := exiftool.NewExiftool()
	if err != nil {
		t.Skipf("Skipping E2E test: exiftool not available: %v", err)
	}
	defer et.Close()
	
	// Process files
	noGpsFile := filepath.Join(tmpDir, "no-gps-data.jpg")
	withGpsFile := filepath.Join(tmpDir, "with-gps-data.jpg")
	
	// Verify initial state
	t.Run("VerifyInitialState", func(t *testing.T) {
		files := et.ExtractMetadata(noGpsFile, withGpsFile)
		
		for _, fileinfo := range files {
			if fileinfo.Err != nil {
				t.Fatalf("Error extracting metadata: %v", fileinfo.Err)
			}
			
			if filepath.Base(fileinfo.File) == "with-gps-data.jpg" {
				// Should already have GPS data
				if fileinfo.Fields["GPSLatitude"] == nil {
					t.Error("with-gps-data.jpg should have GPS latitude")
				}
				if fileinfo.Fields["GPSLongitude"] == nil {
					t.Error("with-gps-data.jpg should have GPS longitude")
				}
			}
		}
	})
	
	// Test processing no-gps-data.jpg
	t.Run("ProcessNoGpsFile", func(t *testing.T) {
		files := et.ExtractMetadata(noGpsFile)
		
		if len(files) != 1 {
			t.Fatalf("Expected 1 file, got %d", len(files))
		}
		
		fileinfo := files[0]
		if fileinfo.Err != nil {
			t.Fatalf("Error extracting metadata: %v", fileinfo.Err)
		}
		
		// Verify no GPS data initially
		if fileinfo.Fields["GPSLatitude"] != nil || fileinfo.Fields["GPSLongitude"] != nil {
			t.Skip("File already has GPS data - skipping modification test")
		}
		
		// Get the datetime
		dateTimeOriginal, err := fileinfo.GetString("DateTimeOriginal")
		if err != nil {
			t.Fatalf("Failed to get DateTimeOriginal: %v", err)
		}
		
		if dateTimeOriginal != "2019:04:19 20:07:30" {
			t.Errorf("Expected DateTimeOriginal '2019:04:19 20:07:30', got '%s'", dateTimeOriginal)
		}
	})
}

func TestE2E_LocationMatching(t *testing.T) {
	// Test that the location matching works with the sample data
	locationFile := "sample_data/Location History/Records.json"
	locations, err := readLocations(locationFile)
	if err != nil {
		t.Fatalf("Failed to read locations: %v", err)
	}
	
	// Setup exiftool to read the photo timestamp
	et, err := exiftool.NewExiftool()
	if err != nil {
		t.Skipf("Skipping E2E test: exiftool not available: %v", err)
	}
	defer et.Close()
	
	noGpsFile := "sample_data/Google Photos/Untitled/no-gps-data.jpg"
	files := et.ExtractMetadata(noGpsFile)
	
	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}
	
	fileinfo := files[0]
	if fileinfo.Err != nil {
		t.Fatalf("Error extracting metadata: %v", fileinfo.Err)
	}
	
	// Parse the datetime from the photo
	dateTimeOriginal, err := fileinfo.GetString("DateTimeOriginal")
	if err != nil {
		t.Fatalf("Failed to get DateTimeOriginal: %v", err)
	}
	
	// The photo was taken at 2019:04:19 20:07:30
	// There should be a location at 2019-04-19T20:08:28.785Z (within 1 hour tolerance)
	// Expected coordinates: latitudeE7: 395107349, longitudeE7: -91427899
	
	// Parse the time to match against locations
	dtParse, err := parseDateTime(dateTimeOriginal)
	if err != nil {
		t.Fatalf("Failed to parse datetime: %v", err)
	}
	
	// Save original tolerance and set test tolerance
	originalTolerance := tolerance
	defer func() { tolerance = originalTolerance }()
	testTolerance := 1 * time.Hour
	tolerance = &testTolerance
	
	location := findLocationFromDate(locations, dtParse)
	if location == nil {
		t.Fatal("Expected to find a location within tolerance")
	}
	
	// Verify the found location matches expected values
	expectedLat := 395107349
	expectedLon := -91427899
	
	if location.LatitudeE7 != expectedLat {
		t.Errorf("Expected latitude %d, got %d", expectedLat, location.LatitudeE7)
	}
	
	if location.LongitudeE7 != expectedLon {
		t.Errorf("Expected longitude %d, got %d", expectedLon, location.LongitudeE7)
	}
	
	// Convert to decimal degrees and verify
	latitude := float32(location.LatitudeE7) / 10000000
	longitude := float32(location.LongitudeE7) / 10000000
	
	expectedLatDeg := float32(39.5107349)
	expectedLonDeg := float32(-9.1427899)
	
	// Allow for small floating point differences
	latDiff := latitude - expectedLatDeg
	if latDiff < -0.0001 || latDiff > 0.0001 {
		t.Errorf("Expected latitude ~%.7f, got %.7f", expectedLatDeg, latitude)
	}
	
	lonDiff := longitude - expectedLonDeg
	if lonDiff < -0.0001 || lonDiff > 0.0001 {
		t.Errorf("Expected longitude ~%.7f, got %.7f", expectedLonDeg, longitude)
	}
}

// Helper function to parse datetime
func parseDateTime(dt string) (time.Time, error) {
	return time.Parse("2006:01:02 15:04:05", dt)
}

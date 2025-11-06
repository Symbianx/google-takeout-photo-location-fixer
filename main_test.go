package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/btree"
)

// Unit tests for core functions

func TestReadLocations(t *testing.T) {
	// Test reading a valid location file
	t.Run("ValidLocationFile", func(t *testing.T) {
		locations, err := readLocations("sample_data/Location History/Records.json")
		if err != nil {
			t.Fatalf("Failed to read locations: %v", err)
		}
		
		if locations.Len() != 5 {
			t.Errorf("Expected 5 locations, got %d", locations.Len())
		}
	})

	// Test reading non-existent file
	t.Run("NonExistentFile", func(t *testing.T) {
		_, err := readLocations("nonexistent.json")
		if err == nil {
			t.Error("Expected error reading non-existent file, got nil")
		}
	})

	// Test reading invalid JSON file
	t.Run("InvalidJSON", func(t *testing.T) {
		// Create a temporary file with invalid JSON
		tmpFile := filepath.Join(t.TempDir(), "invalid.json")
		err := os.WriteFile(tmpFile, []byte("invalid json content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		_, err = readLocations(tmpFile)
		if err == nil {
			t.Error("Expected error reading invalid JSON, got nil")
		}
	})
}

func TestGetUnsignedDateDifference(t *testing.T) {
	tests := []struct {
		name     string
		time1    time.Time
		time2    time.Time
		expected time.Duration
	}{
		{
			name:     "FirstBeforeSecond",
			time1:    time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
			time2:    time.Date(2019, 4, 19, 21, 0, 0, 0, time.UTC),
			expected: 1 * time.Hour,
		},
		{
			name:     "SecondBeforeFirst",
			time1:    time.Date(2019, 4, 19, 21, 0, 0, 0, time.UTC),
			time2:    time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
			expected: 1 * time.Hour,
		},
		{
			name:     "SameTime",
			time1:    time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
			time2:    time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
			expected: 0,
		},
		{
			name:     "MinuteDifference",
			time1:    time.Date(2019, 4, 19, 20, 5, 30, 0, time.UTC),
			time2:    time.Date(2019, 4, 19, 20, 7, 30, 0, time.UTC),
			expected: 2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUnsignedDateDifference(tt.time1, tt.time2)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFindLocationFromDate(t *testing.T) {
	// Create a btree with test locations
	locations := btree.NewG[Location](2, locationLessFunc)
	
	testLocations := []Location{
		{
			LatitudeE7:  258135945,
			LongitudeE7: 81338558,
			Timestamp:   time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
		},
		{
			LatitudeE7:  395107349,
			LongitudeE7: -91427899,
			Timestamp:   time.Date(2019, 4, 19, 20, 8, 28, 785000000, time.UTC),
		},
		{
			LatitudeE7:  258135945,
			LongitudeE7: 81338558,
			Timestamp:   time.Date(2020, 4, 19, 20, 1, 28, 785000000, time.UTC),
		},
	}

	for _, loc := range testLocations {
		locations.ReplaceOrInsert(loc)
	}

	// Save original tolerance
	originalTolerance := tolerance
	defer func() { tolerance = originalTolerance }()
	
	testTolerance := 1 * time.Hour
	tolerance = &testTolerance

	tests := []struct {
		name        string
		searchTime  time.Time
		expectFound bool
		expectedLat int
		expectedLon int
	}{
		{
			name:        "ExactMatch",
			searchTime:  time.Date(2019, 4, 19, 20, 8, 28, 785000000, time.UTC),
			expectFound: true,
			expectedLat: 395107349,
			expectedLon: -91427899,
		},
		{
			name:        "WithinTolerance",
			searchTime:  time.Date(2019, 4, 19, 20, 7, 30, 0, time.UTC),
			expectFound: true,
			expectedLat: 395107349,
			expectedLon: -91427899,
		},
		{
			name:        "OutsideTolerance",
			searchTime:  time.Date(2019, 4, 19, 22, 0, 0, 0, time.UTC),
			expectFound: false,
		},
		{
			name:        "ClosestMatch",
			searchTime:  time.Date(2019, 4, 19, 20, 5, 0, 0, time.UTC),
			expectFound: true,
			expectedLat: 395107349,
			expectedLon: -91427899,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLocationFromDate(locations, tt.searchTime)
			
			if tt.expectFound && result == nil {
				t.Error("Expected to find location, got nil")
			} else if !tt.expectFound && result != nil {
				t.Errorf("Expected no location, got %+v", result)
			}
			
			if tt.expectFound && result != nil {
				if result.LatitudeE7 != tt.expectedLat {
					t.Errorf("Expected latitude %d, got %d", tt.expectedLat, result.LatitudeE7)
				}
				if result.LongitudeE7 != tt.expectedLon {
					t.Errorf("Expected longitude %d, got %d", tt.expectedLon, result.LongitudeE7)
				}
			}
		})
	}
}

func TestLocationLessFunc(t *testing.T) {
	loc1 := Location{
		LatitudeE7:  258135945,
		LongitudeE7: 81338558,
		Timestamp:   time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
	}
	
	loc2 := Location{
		LatitudeE7:  395107349,
		LongitudeE7: -91427899,
		Timestamp:   time.Date(2019, 4, 19, 21, 0, 0, 0, time.UTC),
	}
	
	loc3 := Location{
		LatitudeE7:  258135945,
		LongitudeE7: 81338558,
		Timestamp:   time.Date(2019, 4, 19, 20, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name     string
		a        Location
		b        Location
		expected bool
	}{
		{
			name:     "FirstBeforeSecond",
			a:        loc1,
			b:        loc2,
			expected: true,
		},
		{
			name:     "SecondBeforeFirst",
			a:        loc2,
			b:        loc1,
			expected: false,
		},
		{
			name:     "SameTimestamp",
			a:        loc1,
			b:        loc3,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := locationLessFunc(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

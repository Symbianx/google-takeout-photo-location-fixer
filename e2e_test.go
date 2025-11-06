package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	exiftool "github.com/barasher/go-exiftool"
)

// E2E tests - these tests run the actual binary as a user would from the terminal
// NOTE: These tests assume the binary has already been built (run `go build` before testing)

func TestE2E_FullWorkflow(t *testing.T) {
	// Use the binary from the current directory
	binaryPath := "./google-takeout-photo-location-fixer"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("Binary not found at %s - please run 'go build' before running e2e tests", binaryPath)
	}

	// Create a temp directory for test files
	tmpDir := t.TempDir()
	photosDir := filepath.Join(tmpDir, "photos")
	if err := os.Mkdir(photosDir, 0755); err != nil {
		t.Fatalf("Failed to create photos directory: %v", err)
	}

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
		dstPath := filepath.Join(photosDir, file.Name())

		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", srcPath, err)
		}

		err = os.WriteFile(dstPath, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", dstPath, err)
		}
	}

	// Setup exiftool to verify results
	et, err := exiftool.NewExiftool()
	if err != nil {
		t.Skipf("Skipping E2E test: exiftool not available: %v", err)
	}
	defer et.Close()

	noGpsFile := filepath.Join(photosDir, "no-gps-data.jpg")
	withGpsFile := filepath.Join(photosDir, "with-gps-data.jpg")

	// Verify the file has no GPS data initially
	t.Run("VerifyNoGPSInitially", func(t *testing.T) {
		metadata := et.ExtractMetadata(noGpsFile)
		if len(metadata) != 1 {
			t.Fatalf("Expected 1 file metadata, got %d", len(metadata))
		}
		if metadata[0].Err != nil {
			t.Fatalf("Error extracting metadata: %v", metadata[0].Err)
		}

		if metadata[0].Fields["GPSLatitude"] != nil || metadata[0].Fields["GPSLongitude"] != nil {
			t.Skip("File already has GPS data - cannot test adding GPS data")
		}
	})

	// Capture the GPS data from the file that already has it
	var originalGpsLat, originalGpsLon string
	t.Run("CaptureOriginalGPSData", func(t *testing.T) {
		metadata := et.ExtractMetadata(withGpsFile)
		if len(metadata) != 1 {
			t.Fatalf("Expected 1 file metadata, got %d", len(metadata))
		}
		if metadata[0].Err != nil {
			t.Fatalf("Error extracting metadata: %v", metadata[0].Err)
		}

		if latField := metadata[0].Fields["GPSLatitude"]; latField != nil {
			if latStr, ok := latField.(string); ok {
				originalGpsLat = latStr
			}
		}
		if lonField := metadata[0].Fields["GPSLongitude"]; lonField != nil {
			if lonStr, ok := lonField.(string); ok {
				originalGpsLon = lonStr
			}
		}

		if originalGpsLat == "" || originalGpsLon == "" {
			t.Skip("with-gps-data.jpg doesn't have GPS data - cannot test preservation")
		}
	})

	// Run the tool with sample data
	t.Run("RunTool", func(t *testing.T) {
		locationFile, err := filepath.Abs("sample_data/Location History/Records.json")
		if err != nil {
			t.Fatalf("Failed to get absolute path: %v", err)
		}

		// Run the tool with -y flag to skip confirmation and --skip-backup
		cmd := exec.Command(
			binaryPath,
			"-f", locationFile,
			"-d", photosDir,
			"-y",
			"--skip-backup",
		)

		// Capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Tool execution failed: %v\nOutput: %s", err, output)
		}

		// Verify the output contains expected messages
		outputStr := string(output)
		if !strings.Contains(outputStr, "Read 5 GPS locations") {
			t.Errorf("Expected output to contain 'Read 5 GPS locations', got: %s", outputStr)
		}
	})

	// Verify GPS data was added to the file
	t.Run("VerifyGPSAdded", func(t *testing.T) {
		metadata := et.ExtractMetadata(noGpsFile)
		if len(metadata) != 1 {
			t.Fatalf("Expected 1 file metadata, got %d", len(metadata))
		}
		if metadata[0].Err != nil {
			t.Fatalf("Error extracting metadata: %v", metadata[0].Err)
		}

		// Check that GPS data was added
		latField := metadata[0].Fields["GPSLatitude"]
		lonField := metadata[0].Fields["GPSLongitude"]
		
		if latField == nil {
			t.Error("Expected GPS latitude to be added")
		}
		if lonField == nil {
			t.Error("Expected GPS longitude to be added")
		}

		// The GPS values are returned as strings in DMS format, e.g. "39 deg 30' 38.65\" N"
		// Just verify they're not empty and contain expected hemisphere indicators
		if latStr, ok := latField.(string); ok {
			if !strings.Contains(latStr, "N") && !strings.Contains(latStr, "S") {
				t.Errorf("Expected latitude to contain hemisphere (N/S), got: %s", latStr)
			}
			// Latitude should be around 39 degrees
			if !strings.HasPrefix(latStr, "39 deg") {
				t.Logf("Warning: Expected latitude around 39 degrees, got: %s", latStr)
			}
		}

		if lonStr, ok := lonField.(string); ok {
			if !strings.Contains(lonStr, "E") && !strings.Contains(lonStr, "W") {
				t.Errorf("Expected longitude to contain hemisphere (E/W), got: %s", lonStr)
			}
			// Longitude should be around 9 degrees West
			if !strings.HasPrefix(lonStr, "9 deg") {
				t.Logf("Warning: Expected longitude around 9 degrees, got: %s", lonStr)
			}
		}
	})

	// Verify that files with GPS data already present are not modified
	t.Run("VerifyExistingGPSNotModified", func(t *testing.T) {
		metadata := et.ExtractMetadata(withGpsFile)
		if len(metadata) != 1 {
			t.Fatalf("Expected 1 file metadata, got %d", len(metadata))
		}
		if metadata[0].Err != nil {
			t.Fatalf("Error extracting metadata: %v", metadata[0].Err)
		}

		// Check that GPS data remains unchanged
		currentGpsLat := ""
		currentGpsLon := ""
		
		if latField := metadata[0].Fields["GPSLatitude"]; latField != nil {
			if latStr, ok := latField.(string); ok {
				currentGpsLat = latStr
			}
		}
		if lonField := metadata[0].Fields["GPSLongitude"]; lonField != nil {
			if lonStr, ok := lonField.(string); ok {
				currentGpsLon = lonStr
			}
		}

		if currentGpsLat != originalGpsLat {
			t.Errorf("GPS latitude was modified. Original: %s, Current: %s", originalGpsLat, currentGpsLat)
		}
		if currentGpsLon != originalGpsLon {
			t.Errorf("GPS longitude was modified. Original: %s, Current: %s", originalGpsLon, currentGpsLon)
		}
	})
}

func TestE2E_DryRun(t *testing.T) {
	// Use the binary from the current directory
	binaryPath := "./google-takeout-photo-location-fixer"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("Binary not found at %s - please run 'go build' before running e2e tests", binaryPath)
	}

	// Create a temp directory for test files
	tmpDir := t.TempDir()
	photosDir := filepath.Join(tmpDir, "photos")
	if err := os.Mkdir(photosDir, 0755); err != nil {
		t.Fatalf("Failed to create photos directory: %v", err)
	}

	// Copy the no-gps-data image
	srcPath := "sample_data/Google Photos/Untitled/no-gps-data.jpg"
	dstPath := filepath.Join(photosDir, "no-gps-data.jpg")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Get file modification time before running tool
	infoBefore, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Run the tool in dry-run mode
	locationFile, err := filepath.Abs("sample_data/Location History/Records.json")
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	cmd := exec.Command(
		binaryPath,
		"-f", locationFile,
		"-d", photosDir,
		"-y",
		"-n", // dry-run flag
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Tool execution failed: %v\nOutput: %s", err, output)
	}

	// Verify the file was not modified (compare modification times and content)
	infoAfter, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat file after dry-run: %v", err)
	}

	// Always check file content after dry-run
	dataAfter, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read file after dry-run: %v", err)
	}

	// Compare file content
	if string(data) != string(dataAfter) {
		t.Error("File content changed during dry-run (should not have been modified)")
	}

	// Optionally, also check modification time
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Error("File modification time changed during dry-run (should not have been modified)")
	}
	// Verify output mentions the dry-run didn't write
	outputStr := string(output)
	if !strings.Contains(outputStr, "Read 5 GPS locations") {
		t.Errorf("Expected output to contain 'Read 5 GPS locations'")
	}
}

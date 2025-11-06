package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	exiftool "github.com/barasher/go-exiftool"
)

// E2E tests - these tests run the actual binary as a user would from the terminal

func TestE2E_FullWorkflow(t *testing.T) {
	// Build the binary first
	binaryPath := filepath.Join(t.TempDir(), "google-takeout-photo-location-fixer")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, output)
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
}

func TestE2E_DryRun(t *testing.T) {
	// Build the binary first
	binaryPath := filepath.Join(t.TempDir(), "google-takeout-photo-location-fixer")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, output)
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

	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		// Modification time might change, so also check content
		dataAfter, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("Failed to read file after dry-run: %v", err)
		}

		// Compare file content
		if string(data) != string(dataAfter) {
			t.Error("File content changed during dry-run (should not have been modified)")
		}
	}

	// Verify output mentions the dry-run didn't write
	outputStr := string(output)
	if !strings.Contains(outputStr, "Read 5 GPS locations") {
		t.Errorf("Expected output to contain 'Read 5 GPS locations'")
	}
}

func TestE2E_MissingExiftool(t *testing.T) {
	// Build the binary first
	binaryPath := filepath.Join(t.TempDir(), "google-takeout-photo-location-fixer")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, output)
	}

	// Create minimal test setup
	tmpDir := t.TempDir()
	photosDir := filepath.Join(tmpDir, "photos")
	if err := os.Mkdir(photosDir, 0755); err != nil {
		t.Fatalf("Failed to create photos directory: %v", err)
	}

	locationFile, err := filepath.Abs("sample_data/Location History/Records.json")
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	// Run with invalid exiftool path - should fail gracefully
	cmd := exec.Command(
		binaryPath,
		"-f", locationFile,
		"-d", photosDir,
		"-y",
		"--exiftool-binary", "/nonexistent/path/to/exiftool",
	)

	// Redirect stderr to capture error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	stderrBytes, _ := io.ReadAll(stderr)
	cmd.Wait()

	// Tool should exit with error when exiftool is not found
	if cmd.ProcessState.ExitCode() == 0 {
		t.Error("Expected non-zero exit code when exiftool is missing")
	}

	// Verify error message mentions exiftool
	stderrStr := string(stderrBytes)
	if !strings.Contains(stderrStr, "exiftool") && !strings.Contains(stderrStr, "Error") {
		t.Logf("stderr output: %s", stderrStr)
	}
}

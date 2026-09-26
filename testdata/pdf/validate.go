package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type manifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	Fixtures      []fixture `json:"fixtures"`
}

type fixture struct {
	Path                   string   `json:"path"`
	SHA256                 string   `json:"sha256"`
	Purpose                string   `json:"purpose"`
	Features               []string `json:"features"`
	Origin                 string   `json:"origin"`
	GenerationSource       string   `json:"generationSource"`
	GenerationInstructions string   `json:"generationInstructions"`
	Author                 string   `json:"author"`
	Copyright              string   `json:"copyright"`
	License                string   `json:"license"`
	ExpectedPageCount      int      `json:"expectedPageCount"`
	ExpectedText           string   `json:"expectedText"`
}

func main() {
	manifestPath := flag.String("manifest", "testdata/pdf/manifest.json", "fixture manifest path")
	flag.Parse()

	if err := validate(*manifestPath); err != nil {
		fmt.Fprintf(os.Stderr, "validate PDF fixtures: %v\n", err)
		os.Exit(1)
	}
}

func validate(manifestPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	var contents manifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contents); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if contents.SchemaVersion != 1 {
		return fmt.Errorf("schema version must be 1, got %d", contents.SchemaVersion)
	}
	if len(contents.Fixtures) == 0 {
		return errors.New("manifest must contain at least one fixture")
	}

	root, err := filepath.Abs(filepath.Dir(manifestPath))
	if err != nil {
		return fmt.Errorf("resolve manifest directory: %w", err)
	}
	for i, fixture := range contents.Fixtures {
		if err := validateFixture(root, fixture); err != nil {
			return fmt.Errorf("fixture %d: %w", i+1, err)
		}
	}
	return nil
}

func validateFixture(root string, fixture fixture) error {
	if fixture.Path == "" || fixture.Purpose == "" || len(fixture.Features) == 0 {
		return errors.New("path, purpose, and features are required")
	}
	if fixture.Origin == "" || fixture.GenerationSource == "" || fixture.GenerationInstructions == "" {
		return errors.New("origin and generation details are required")
	}
	if fixture.Author == "" || fixture.Copyright == "" || fixture.License == "" {
		return errors.New("author, copyright, and license are required")
	}
	if fixture.ExpectedPageCount <= 0 || fixture.ExpectedText == "" {
		return errors.New("expected page count and text are required")
	}

	pdfPath, err := resolveWithin(root, fixture.Path)
	if err != nil {
		return fmt.Errorf("path: %w", err)
	}
	sourcePath, err := resolveWithin(root, fixture.GenerationSource)
	if err != nil {
		return fmt.Errorf("generation source: %w", err)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("stat generation source: %w", err)
	}

	wantDigest, err := hex.DecodeString(fixture.SHA256)
	if err != nil || len(wantDigest) != sha256.Size {
		return errors.New("sha256 must be a 64-character hexadecimal digest")
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return fmt.Errorf("read PDF: %w", err)
	}
	gotDigest := sha256.Sum256(data)
	if !bytes.Equal(gotDigest[:], wantDigest) {
		return fmt.Errorf("sha256 mismatch: got %x, want %s", gotDigest, fixture.SHA256)
	}
	return nil
}

func resolveWithin(root, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", errors.New("must be relative")
	}
	path := filepath.Join(root, filepath.Clean(relativePath))
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("must remain within the fixture directory")
	}
	return path, nil
}

package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
)

func TestWriterDestinationWritesRenderedContent(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	destination, err := app.NewWriterDestination(&output)
	if err != nil {
		t.Fatalf("NewWriterDestination() returned an unexpected error: %v", err)
	}

	if err := destination.Commit(context.Background(), []byte("Markdown\n")); err != nil {
		t.Fatalf("Commit() returned an unexpected error: %v", err)
	}
	if got, want := output.String(), "Markdown\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriterDestinationReportsWriteFailures(t *testing.T) {
	t.Parallel()

	want := errors.New("write failed")
	tests := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "writer error", writer: outputErrorWriter{err: want}, want: want},
		{name: "short write", writer: outputShortWriter{}, want: io.ErrShortWrite},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			destination, err := app.NewWriterDestination(test.writer)
			if err != nil {
				t.Fatalf("NewWriterDestination() returned an unexpected error: %v", err)
			}
			err = destination.Commit(context.Background(), []byte("content"))
			if !errors.Is(err, test.want) {
				t.Fatalf("Commit() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestFileDestinationCreatesOutputAndRemovesTemporaryFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "result.md")
	destination, err := app.NewFileDestination(path, false)
	if err != nil {
		t.Fatalf("NewFileDestination() returned an unexpected error: %v", err)
	}

	if err := destination.Commit(context.Background(), []byte("Markdown\n")); err != nil {
		t.Fatalf("Commit() returned an unexpected error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if got, want := string(content), "Markdown\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	assertDirectoryEntries(t, directory, []string{"result.md"})
}

func TestFileDestinationPreservesExistingOutputWithoutForce(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "result.md")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	destination, err := app.NewFileDestination(path, false)
	if err != nil {
		t.Fatalf("NewFileDestination() returned an unexpected error: %v", err)
	}

	err = destination.Commit(context.Background(), []byte("replacement"))
	if !errors.Is(err, app.ErrOutputExists) {
		t.Fatalf("Commit() error = %v, want %v", err, app.ErrOutputExists)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read existing output: %v", readErr)
	}
	if got, want := string(content), "original"; got != want {
		t.Fatalf("existing output = %q, want %q", got, want)
	}
	assertDirectoryEntries(t, directory, []string{"result.md"})
}

func TestFileDestinationReplacesExistingOutputWithForce(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "result.md")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	destination, err := app.NewFileDestination(path, true)
	if err != nil {
		t.Fatalf("NewFileDestination() returned an unexpected error: %v", err)
	}

	if err := destination.Commit(context.Background(), []byte("replacement")); err != nil {
		t.Fatalf("Commit() returned an unexpected error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replaced output: %v", err)
	}
	if got, want := string(content), "replacement"; got != want {
		t.Fatalf("replaced output = %q, want %q", got, want)
	}
	assertDirectoryEntries(t, directory, []string{"result.md"})
}

func TestFileDestinationDoesNotCommitCanceledOutput(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "result.md")
	destination, err := app.NewFileDestination(path, false)
	if err != nil {
		t.Fatalf("NewFileDestination() returned an unexpected error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = destination.Commit(ctx, []byte("content"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Commit() error = %v, want %v", err, context.Canceled)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output stat error = %v, want not-exist error", statErr)
	}
	assertDirectoryEntries(t, directory, nil)
}

func TestFileDestinationCleansUpWhenCanceledDuringWrite(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "result.md")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	destination, err := app.NewFileDestination(path, true)
	if err != nil {
		t.Fatalf("NewFileDestination() returned an unexpected error: %v", err)
	}
	ctx := &cancelAfterChecksContext{
		Context:  context.Background(),
		cancelAt: 3,
	}

	err = destination.Commit(ctx, bytes.Repeat([]byte("x"), 64*1024))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Commit() error = %v, want %v", err, context.Canceled)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read existing output: %v", readErr)
	}
	if got, want := string(content), "original"; got != want {
		t.Fatalf("existing output = %q, want %q", got, want)
	}
	assertDirectoryEntries(t, directory, []string{"result.md"})
}

func TestFileDestinationRejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	outputDirectory := filepath.Join(directory, "output")
	if err := os.Mkdir(outputDirectory, 0o755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	tests := []struct {
		name string
		path string
	}{
		{
			name: "missing parent",
			path: filepath.Join(directory, "missing", "result.md"),
		},
		{
			name: "output is directory",
			path: outputDirectory,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			destination, err := app.NewFileDestination(test.path, true)
			if err != nil {
				t.Fatalf("NewFileDestination() returned an unexpected error: %v", err)
			}
			if err := destination.Commit(context.Background(), []byte("content")); err == nil {
				t.Fatal("Commit() returned nil error for an invalid path")
			}
		})
	}
}

func TestDestinationConstructorsRejectEmptyValues(t *testing.T) {
	t.Parallel()

	if _, err := app.NewWriterDestination(nil); err == nil {
		t.Fatal("NewWriterDestination() returned nil error for a nil writer")
	}
	if _, err := app.NewFileDestination("", false); err == nil {
		t.Fatal("NewFileDestination() returned nil error for an empty path")
	}
}

type outputErrorWriter struct {
	err error
}

func (writer outputErrorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type outputShortWriter struct{}

func (outputShortWriter) Write(content []byte) (int, error) {
	return len(content) - 1, nil
}

type cancelAfterChecksContext struct {
	context.Context
	checks   int
	cancelAt int
}

func (ctx *cancelAfterChecksContext) Err() error {
	ctx.checks++
	if ctx.checks >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func assertDirectoryEntries(t *testing.T, directory string, want []string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Name()
	}
	if len(got) != len(want) {
		t.Fatalf("directory entries = %#v, want %#v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("directory entries = %#v, want %#v", got, want)
		}
	}
}

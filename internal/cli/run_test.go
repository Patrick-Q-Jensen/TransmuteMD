package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

func TestRunnerWritesStdoutAfterClosingResources(t *testing.T) {
	t.Parallel()

	input := writePDFInput(t, "report.pdf")
	var events []string
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newTestRunner(
		t,
		func(context.Context) (Converter, io.Closer, error) {
			return converterFunc(func(
					ctx context.Context,
					source extract.Source,
					destination app.Destination,
				) error {
					events = append(events, "convert")
					if source.Size() == 0 {
						t.Fatal("source size is zero")
					}
					return destination.Commit(ctx, []byte("Markdown\n"))
				}), closerFunc(func() error {
					events = append(events, "close")
					return nil
				}), nil
		},
		eventWriter{
			events: &events,
			writer: &stdout,
		},
		&stderr,
	)

	code := runner.Run(context.Background(), []string{input, "--output", "-"})
	if code != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", code, ExitSuccess, stderr.String())
	}
	if got, want := stdout.String(), "Markdown\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no diagnostics", stderr.String())
	}
	if want := []string{"convert", "close", "write"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestRunnerWritesDefaultAndExplicitFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputArgs func(string) []string
		outputPath func(string) string
	}{
		{
			name:       "default",
			outputArgs: func(string) []string { return nil },
			outputPath: func(input string) string {
				return strings.TrimSuffix(input, filepath.Ext(input)) + ".md"
			},
		},
		{
			name: "explicit",
			outputArgs: func(input string) []string {
				return []string{"--output", filepath.Join(filepath.Dir(input), "explicit.md")}
			},
			outputPath: func(input string) string {
				return filepath.Join(filepath.Dir(input), "explicit.md")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := writePDFInput(t, "report.pdf")
			runner := successfulRunner(t, "Markdown\n")
			arguments := append([]string{input}, test.outputArgs(input)...)

			if code := runner.Run(context.Background(), arguments); code != ExitSuccess {
				t.Fatalf("Run() exit code = %d, want %d", code, ExitSuccess)
			}
			output, err := os.ReadFile(test.outputPath(input))
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if got, want := string(output), "Markdown\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestRunnerPreservesExistingFileUnlessForced(t *testing.T) {
	t.Parallel()

	input := writePDFInput(t, "report.pdf")
	output := filepath.Join(filepath.Dir(input), "result.md")
	if err := os.WriteFile(output, []byte("original"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}
	runner := successfulRunner(t, "replacement")

	code := runner.Run(
		context.Background(),
		[]string{input, "--output", output},
	)
	if code != ExitOutput {
		t.Fatalf("Run() exit code = %d, want %d", code, ExitOutput)
	}
	assertFileContent(t, output, "original")

	code = runner.Run(
		context.Background(),
		[]string{input, "--output", output, "--force"},
	)
	if code != ExitSuccess {
		t.Fatalf("Run() with force exit code = %d, want %d", code, ExitSuccess)
	}
	assertFileContent(t, output, "replacement")
}

func TestRunnerRejectsInputAndOutputResolvingToSameFile(t *testing.T) {
	t.Parallel()

	input := writePDFInput(t, "report.pdf")
	factoryCalled := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newTestRunner(
		t,
		func(context.Context) (Converter, io.Closer, error) {
			factoryCalled = true
			return nil, nil, errors.New("must not be called")
		},
		&stdout,
		&stderr,
	)

	code := runner.Run(
		context.Background(),
		[]string{input, "--output", input, "--force"},
	)
	if code != ExitOutput {
		t.Fatalf("Run() exit code = %d, want %d", code, ExitOutput)
	}
	if factoryCalled {
		t.Fatal("converter factory called for identical input and output")
	}
	assertFileContent(t, input, "%PDF-1.4\n")
}

func TestRunnerRejectsUnrecognizedInputBeforeCreatingOutput(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	if err := os.WriteFile(input, []byte("not a PDF"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	output := filepath.Join(directory, "output.md")
	factoryCalled := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := newTestRunner(
		t,
		func(context.Context) (Converter, io.Closer, error) {
			factoryCalled = true
			return nil, nil, errors.New("must not be called")
		},
		&stdout,
		&stderr,
	)

	code := runner.Run(context.Background(), []string{input, "--output", output})
	if code != ExitInput {
		t.Fatalf("Run() exit code = %d, want %d", code, ExitInput)
	}
	if factoryCalled {
		t.Fatal("converter factory called for unrecognized input")
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output stat error = %v, want not-exist error", err)
	}
}

func TestRunnerMapsConversionAndCancellationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "conversion", err: errors.New("conversion failed"), code: ExitConversion},
		{name: "cancellation", err: context.Canceled, code: ExitInterrupted},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := writePDFInput(t, "report.pdf")
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			runner := newTestRunner(
				t,
				func(context.Context) (Converter, io.Closer, error) {
					return converterFunc(func(
						context.Context,
						extract.Source,
						app.Destination,
					) error {
						return test.err
					}), closerFunc(func() error { return nil }), nil
				},
				&stdout,
				&stderr,
			)

			code := runner.Run(
				context.Background(),
				[]string{input, "--output", "-"},
			)
			if code != test.code {
				t.Fatalf("Run() exit code = %d, want %d", code, test.code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no output", stdout.String())
			}
			if stderr.Len() == 0 {
				t.Fatal("stderr is empty, want diagnostic")
			}
		})
	}
}

func TestRunnerDoesNotCreateFileAfterConversionFailure(t *testing.T) {
	t.Parallel()

	input := writePDFInput(t, "report.pdf")
	output := filepath.Join(filepath.Dir(input), "result.md")
	runner := newTestRunner(
		t,
		func(context.Context) (Converter, io.Closer, error) {
			return converterFunc(func(
				context.Context,
				extract.Source,
				app.Destination,
			) error {
				return errors.New("conversion failed")
			}), closerFunc(func() error { return nil }), nil
		},
		io.Discard,
		io.Discard,
	)

	code := runner.Run(context.Background(), []string{input, "--output", output})
	if code != ExitConversion {
		t.Fatalf("Run() exit code = %d, want %d", code, ExitConversion)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output stat error = %v, want not-exist error", err)
	}
}

func TestRunnerDoesNotWriteWhenResourceCleanupFails(t *testing.T) {
	t.Parallel()

	input := writePDFInput(t, "report.pdf")
	closeErr := errors.New("close failed")
	var stdout bytes.Buffer
	runner := newTestRunner(
		t,
		func(context.Context) (Converter, io.Closer, error) {
			return converterFunc(func(
				ctx context.Context,
				_ extract.Source,
				destination app.Destination,
			) error {
				return destination.Commit(ctx, []byte("must not be written"))
			}), closerFunc(func() error { return closeErr }), nil
		},
		&stdout,
		io.Discard,
	)

	code := runner.Run(context.Background(), []string{input, "--output", "-"})
	if code != ExitConversion {
		t.Fatalf("Run() exit code = %d, want %d", code, ExitConversion)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no output", stdout.String())
	}
}

func TestRunnerHandlesHelpVersionAndUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
		code      int
		stdout    string
		stderr    bool
	}{
		{
			name:      "help",
			arguments: []string{"--help"},
			code:      ExitSuccess,
			stdout:    usageText,
		},
		{
			name:      "version",
			arguments: []string{"--version"},
			code:      ExitSuccess,
			stdout:    programName + " " + Version + "\n",
		},
		{
			name:      "usage error",
			arguments: []string{"--unknown"},
			code:      ExitUsage,
			stderr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			factoryCalled := false
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			runner := newTestRunner(
				t,
				func(context.Context) (Converter, io.Closer, error) {
					factoryCalled = true
					return nil, nil, errors.New("must not be called")
				},
				&stdout,
				&stderr,
			)

			if code := runner.Run(context.Background(), test.arguments); code != test.code {
				t.Fatalf("Run() exit code = %d, want %d", code, test.code)
			}
			if stdout.String() != test.stdout {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.stdout)
			}
			if got := stderr.Len() > 0; got != test.stderr {
				t.Fatalf("stderr present = %t, want %t; stderr = %q", got, test.stderr, stderr.String())
			}
			if factoryCalled {
				t.Fatal("converter factory called for non-conversion command")
			}
		})
	}
}

func newTestRunner(
	t *testing.T,
	factory ConverterFactory,
	stdout io.Writer,
	stderr io.Writer,
) *Runner {
	t.Helper()

	runner, err := NewRunner(factory, stdout, stderr)
	if err != nil {
		t.Fatalf("NewRunner() returned an unexpected error: %v", err)
	}
	return runner
}

func successfulRunner(t *testing.T, content string) *Runner {
	t.Helper()

	return newTestRunner(
		t,
		func(context.Context) (Converter, io.Closer, error) {
			return converterFunc(func(
				ctx context.Context,
				_ extract.Source,
				destination app.Destination,
			) error {
				return destination.Commit(ctx, []byte(content))
			}), closerFunc(func() error { return nil }), nil
		},
		io.Discard,
		io.Discard,
	)
}

func writePDFInput(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatalf("write PDF input: %v", err)
	}
	return path
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if got := string(content); got != want {
		t.Fatalf("%q content = %q, want %q", path, got, want)
	}
}

type converterFunc func(context.Context, extract.Source, app.Destination) error

func (function converterFunc) Convert(
	ctx context.Context,
	source extract.Source,
	destination app.Destination,
) error {
	return function(ctx, source, destination)
}

type closerFunc func() error

func (function closerFunc) Close() error {
	return function()
}

type eventWriter struct {
	events *[]string
	writer io.Writer
}

func (writer eventWriter) Write(content []byte) (int, error) {
	*writer.events = append(*writer.events, "write")
	return writer.writer.Write(content)
}

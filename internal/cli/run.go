package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

const (
	ExitSuccess     = 0
	ExitUsage       = 2
	ExitInput       = 3
	ExitConversion  = 4
	ExitOutput      = 5
	ExitInterrupted = 130

	Version = "dev"
)

var (
	errNilFactory         = errors.New("converter factory must not be nil")
	errNilStandardOut     = errors.New("standard output writer must not be nil")
	errNilStandardErr     = errors.New("standard error writer must not be nil")
	errInput              = errors.New("input operation failed")
	errUnsupportedPDF     = errors.New("input is not recognized as a PDF")
	errConversionResource = errors.New("conversion resource cleanup failed")
)

// Converter runs one conversion using caller-owned input and output.
type Converter interface {
	ConvertWithResult(
		ctx context.Context,
		source extract.Source,
		destination app.Destination,
	) (app.Result, error)
}

// ConverterFactory creates one converter and its process-scoped resources.
type ConverterFactory func(ctx context.Context) (Converter, io.Closer, error)

// Runner owns command-line parsing, diagnostics, and exit-code mapping.
type Runner struct {
	factory ConverterFactory
	stdout  io.Writer
	stderr  io.Writer
}

// NewRunner creates a CLI runner.
func NewRunner(
	factory ConverterFactory,
	stdout io.Writer,
	stderr io.Writer,
) (*Runner, error) {
	if factory == nil {
		return nil, errNilFactory
	}
	if stdout == nil {
		return nil, errNilStandardOut
	}
	if stderr == nil {
		return nil, errNilStandardErr
	}
	return &Runner{factory: factory, stdout: stdout, stderr: stderr}, nil
}

// Run executes one command and returns its process exit code.
func (runner *Runner) Run(ctx context.Context, arguments []string) int {
	parsed, err := parseOptions(arguments)
	if err != nil {
		runner.writeDiagnostic(err)
		return ExitUsage
	}
	if parsed.showHelp {
		if _, err := io.WriteString(runner.stdout, usageText); err != nil {
			runner.writeDiagnostic(fmt.Errorf("write help: %w", err))
			return ExitOutput
		}
		return ExitSuccess
	}
	if parsed.showVersion {
		if _, err := fmt.Fprintf(runner.stdout, "%s %s\n", programName, Version); err != nil {
			runner.writeDiagnostic(fmt.Errorf("write version: %w", err))
			return ExitOutput
		}
		return ExitSuccess
	}

	result, err := runner.convert(ctx, parsed)
	if err == nil {
		for _, diagnostic := range result.Diagnostics {
			runner.writeWarning(diagnostic)
		}
		return ExitSuccess
	}
	runner.writeDiagnostic(err)
	return exitCode(err)
}

func (runner *Runner) convert(
	ctx context.Context,
	parsed options,
) (result app.Result, resultErr error) {
	source, inputInfo, err := openPDFSource(parsed.input)
	if err != nil {
		return app.Result{}, fmt.Errorf("%w: %w", errInput, err)
	}

	resources := &resourceGroup{}
	resources.add(source)
	defer func() {
		if !resources.closed {
			resultErr = errors.Join(resultErr, resources.Close())
		}
	}()

	destination, err := runner.destination(parsed, inputInfo)
	if err != nil {
		return app.Result{}, err
	}

	converter, closer, err := runner.factory(ctx)
	if closer != nil {
		resources.add(closer)
	}
	if err != nil {
		return app.Result{}, fmt.Errorf("initialize conversion pipeline: %w", err)
	}
	if converter == nil {
		return app.Result{}, errors.New("initialize conversion pipeline: factory returned a nil converter")
	}

	transactional := &resourceDestination{
		resources:   resources,
		destination: destination,
	}
	return converter.ConvertWithResult(ctx, source, transactional)
}

func (runner *Runner) destination(
	parsed options,
	inputInfo os.FileInfo,
) (app.Destination, error) {
	if parsed.outputSet && parsed.output == "-" {
		destination, err := app.NewWriterDestination(runner.stdout)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", app.ErrOutput, err)
		}
		return destination, nil
	}

	same, err := pathsReferToSameFile(parsed.input, parsed.output, inputInfo)
	if err != nil {
		return nil, fmt.Errorf("%w: compare input and output paths: %w", app.ErrOutput, err)
	}
	if same {
		return nil, fmt.Errorf("%w: output path must not resolve to the input file", app.ErrOutput)
	}

	destination, err := app.NewFileDestination(parsed.output, parsed.force)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", app.ErrOutput, err)
	}
	return destination, nil
}

func (runner *Runner) writeDiagnostic(err error) {
	_, _ = fmt.Fprintf(runner.stderr, "%s: %v\n", programName, err)
}

func (runner *Runner) writeWarning(diagnostic document.Diagnostic) {
	location := ""
	if diagnostic.Page > 0 {
		location = fmt.Sprintf("page %d: ", diagnostic.Page)
	}
	_, _ = fmt.Fprintf(
		runner.stderr,
		"%s: warning: %s%s\n",
		programName,
		location,
		diagnostic.Message,
	)
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, context.Canceled):
		return ExitInterrupted
	case errors.Is(err, errInput):
		return ExitInput
	case errors.Is(err, errConversionResource):
		return ExitConversion
	case errors.Is(err, app.ErrOutput), errors.Is(err, app.ErrOutputExists):
		return ExitOutput
	default:
		return ExitConversion
	}
}

type fileSource struct {
	*os.File
	size int64
}

func (source *fileSource) Size() int64 {
	return source.size
}

func openPDFSource(path string) (*fileSource, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %q: %w", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("inspect %q: %w", path, err),
			closeError(file),
		)
	}
	if info.IsDir() {
		return nil, nil, errors.Join(
			fmt.Errorf("input %q is a directory", path),
			closeError(file),
		)
	}
	if err := recognizePDF(file, info.Size()); err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("inspect input format: %w", err),
			closeError(file),
		)
	}
	return &fileSource{File: file, size: info.Size()}, info, nil
}

func recognizePDF(source io.ReaderAt, size int64) error {
	const headerLimit = 1024
	if size <= 0 {
		return errUnsupportedPDF
	}

	header := make([]byte, min(size, headerLimit))
	read, err := source.ReadAt(header, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if !bytes.Contains(header[:read], []byte("%PDF-")) {
		return errUnsupportedPDF
	}
	return nil
}

func pathsReferToSameFile(
	input string,
	output string,
	inputInfo os.FileInfo,
) (bool, error) {
	outputInfo, err := os.Stat(output)
	if err == nil {
		return os.SameFile(inputInfo, outputInfo), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	inputAbsolute, err := filepath.Abs(input)
	if err != nil {
		return false, err
	}
	outputAbsolute, err := filepath.Abs(output)
	if err != nil {
		return false, err
	}
	inputResolved, err := filepath.EvalSymlinks(inputAbsolute)
	if err != nil {
		return false, err
	}
	outputParent, err := filepath.EvalSymlinks(filepath.Dir(outputAbsolute))
	if err != nil {
		return false, err
	}
	outputResolved := filepath.Join(outputParent, filepath.Base(outputAbsolute))
	return sameCleanPath(inputResolved, outputResolved), nil
}

func sameCleanPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

type resourceGroup struct {
	closers  []io.Closer
	closed   bool
	closeErr error
}

func (group *resourceGroup) add(closer io.Closer) {
	group.closers = append(group.closers, closer)
}

func (group *resourceGroup) Close() error {
	if group.closed {
		return group.closeErr
	}
	group.closed = true
	for index := len(group.closers) - 1; index >= 0; index-- {
		if err := group.closers[index].Close(); err != nil {
			group.closeErr = errors.Join(
				group.closeErr,
				fmt.Errorf("close conversion resource: %w", err),
			)
		}
	}
	return group.closeErr
}

type resourceDestination struct {
	resources   *resourceGroup
	destination app.Destination
}

func (destination *resourceDestination) Commit(
	ctx context.Context,
	content []byte,
) error {
	if err := destination.resources.Close(); err != nil {
		return fmt.Errorf("%w: %w", errConversionResource, err)
	}
	return destination.destination.Commit(ctx, content)
}

func closeError(closer io.Closer) error {
	if err := closer.Close(); err != nil {
		return fmt.Errorf("close input: %w", err)
	}
	return nil
}

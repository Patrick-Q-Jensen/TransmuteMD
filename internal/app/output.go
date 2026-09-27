package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const writeChunkSize = 32 * 1024

var (
	errNilWriter = errors.New("destination writer must not be nil")
	errEmptyPath = errors.New("destination path must not be empty")

	// ErrOutputExists indicates that replacement was not enabled for an
	// existing output file.
	ErrOutputExists = errors.New("output file already exists")
)

// Destination commits fully rendered Markdown to user-visible output.
// Implementations must not retain content after Commit returns.
type Destination interface {
	Commit(ctx context.Context, content []byte) error
}

// WriterDestination commits output to a caller-owned writer such as stdout.
type WriterDestination struct {
	writer io.Writer
}

// NewWriterDestination creates a destination that does not close writer.
func NewWriterDestination(writer io.Writer) (*WriterDestination, error) {
	if writer == nil {
		return nil, errNilWriter
	}
	return &WriterDestination{writer: writer}, nil
}

// Commit writes fully rendered content to the destination writer.
func (destination *WriterDestination) Commit(
	ctx context.Context,
	content []byte,
) error {
	if destination == nil || destination.writer == nil {
		return errNilWriter
	}
	if err := writeContent(ctx, destination.writer, content); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// FileDestination commits output through a temporary file in the destination
// directory.
type FileDestination struct {
	path  string
	force bool
}

// NewFileDestination creates a destination for path. When force is false,
// Commit rejects an existing output file.
func NewFileDestination(path string, force bool) (*FileDestination, error) {
	if path == "" {
		return nil, errEmptyPath
	}
	return &FileDestination{path: filepath.Clean(path), force: force}, nil
}

// Commit writes, synchronizes, and closes a temporary file before replacing
// the destination path.
func (destination *FileDestination) Commit(
	ctx context.Context,
	content []byte,
) (resultErr error) {
	if destination == nil || destination.path == "" {
		return errEmptyPath
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	mode, err := destinationMode(destination.path, destination.force)
	if err != nil {
		return err
	}

	parent := filepath.Dir(destination.path)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("inspect output directory %q: %w", parent, err)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("output parent %q is not a directory", parent)
	}

	temporary, err := os.CreateTemp(
		parent,
		"."+filepath.Base(destination.path)+".tmp-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary output beside %q: %w", destination.path, err)
	}
	temporaryPath := temporary.Name()
	temporaryOpen := true
	temporaryExists := true
	defer func() {
		if temporaryOpen {
			resultErr = errors.Join(
				resultErr,
				wrapCleanupError("close temporary output", temporary.Close()),
			)
		}
		if temporaryExists {
			resultErr = errors.Join(
				resultErr,
				wrapCleanupError("remove temporary output", os.Remove(temporaryPath)),
			)
		}
	}()

	if err := writeContent(ctx, temporary, content); err != nil {
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary output permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("synchronize temporary output: %w", err)
	}

	closeErr := temporary.Close()
	temporaryOpen = false
	if closeErr != nil {
		return fmt.Errorf("close temporary output: %w", closeErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !destination.force {
		if _, err := os.Lstat(destination.path); err == nil {
			return fmt.Errorf("%w: %q", ErrOutputExists, destination.path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("recheck output path %q: %w", destination.path, err)
		}
	}
	if err := os.Rename(temporaryPath, destination.path); err != nil {
		return fmt.Errorf("replace output %q: %w", destination.path, err)
	}
	temporaryExists = false
	return nil
}

func destinationMode(path string, force bool) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return 0, fmt.Errorf("output path %q is a directory", path)
		}
		if !force {
			return 0, fmt.Errorf("%w: %q", ErrOutputExists, path)
		}
		return info.Mode().Perm(), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect output path %q: %w", path, err)
	}
	return 0o644, nil
}

func writeContent(ctx context.Context, output io.Writer, content []byte) error {
	for len(content) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}

		chunkLength := min(len(content), writeChunkSize)
		written, err := output.Write(content[:chunkLength])
		if err != nil {
			return err
		}
		if written != chunkLength {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}

func wrapCleanupError(operation string, err error) error {
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

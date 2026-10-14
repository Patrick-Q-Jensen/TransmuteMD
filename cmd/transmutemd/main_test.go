package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/cli"
)

func TestExecutableSmoke(t *testing.T) {
	root := smokeRepositoryRoot(t)
	executable := smokeExecutable(t, root)
	fixtures := filepath.Join(root, "testdata", "pdf", "generated")

	t.Run("help-and-version", func(t *testing.T) {
		directory := t.TempDir()
		stdout, stderr := runSmokeCLI(t, executable, directory, cli.ExitSuccess, "--help")
		if !strings.HasPrefix(stdout, "Usage: transmutemd ") || stderr != "" {
			t.Fatalf("help streams = %q, %q", stdout, stderr)
		}
		stdout, stderr = runSmokeCLI(t, executable, directory, cli.ExitSuccess, "--version")
		version := "dev"
		if expected, ok := os.LookupEnv("TRANSMUTEMD_SMOKE_EXPECTED_VERSION"); ok {
			if expected == "" {
				t.Fatal("TRANSMUTEMD_SMOKE_EXPECTED_VERSION must not be empty")
			}
			version = expected
		}
		if want := "transmutemd " + version + "\n"; stdout != want || stderr != "" {
			t.Fatalf("version streams = %q, %q; want %q, empty stderr", stdout, stderr, want)
		}
		assertSmokeFiles(t, directory)
	})

	t.Run("goldens", func(t *testing.T) {
		for _, fixture := range []struct {
			name     string
			warnings string
		}{
			{name: "simple"},
			{
				name: "phase2",
				warnings: "transmutemd: warning: page 3: table-like layout was preserved as plain text\n" +
					"transmutemd: warning: page 3: code-like layout was preserved as plain text\n",
			},
			{name: "contents"},
		} {
			t.Run(fixture.name, func(t *testing.T) {
				input := readSmokeFile(t, filepath.Join(fixtures, fixture.name+".pdf"))
				want := readSmokeFile(t, filepath.Join(fixtures, fixture.name+".md.golden"))
				for _, output := range []string{"stdout", "explicit", "default"} {
					t.Run(output, func(t *testing.T) {
						directory := t.TempDir()
						writeSmokeFile(t, filepath.Join(directory, "input document.bin"), input, 0o600)
						args := []string{"input document.bin"}
						files := []string{"input document.bin"}
						switch output {
						case "stdout":
							args = append(args, "--output", "-")
						case "explicit":
							args = append(args, "--output", "result document.md")
							files = append(files, "result document.md")
						case "default":
							files = append(files, "input document.md")
						}
						stdout, stderr := runSmokeCLI(t, executable, directory, cli.ExitSuccess, args...)
						if stderr != fixture.warnings {
							t.Fatalf("stderr = %q, want %q", stderr, fixture.warnings)
						}
						if output == "stdout" {
							if !bytes.Equal([]byte(stdout), want) {
								t.Fatalf("stdout = %q, want %q", stdout, want)
							}
						} else {
							if stdout != "" {
								t.Fatalf("file conversion stdout = %q, want empty", stdout)
							}
							assertSmokeFileContent(t, filepath.Join(directory, files[1]), want)
						}
						assertSmokeFileContent(t, filepath.Join(directory, files[0]), input)
						assertSmokeFiles(t, directory, files...)
					})
				}
			})
		}
	})

	t.Run("overwrite-protection", func(t *testing.T) {
		directory := t.TempDir()
		input := readSmokeFile(t, filepath.Join(fixtures, "simple.pdf"))
		want := readSmokeFile(t, filepath.Join(fixtures, "simple.md.golden"))
		writeSmokeFile(t, filepath.Join(directory, "input.pdf"), input, 0o600)
		output := filepath.Join(directory, "output.md")
		original := []byte("existing user output\r\n")
		writeSmokeFile(t, output, original, 0o600)

		runSmokeCLI(t, executable, directory, cli.ExitOutput, "input.pdf", "-o", "output.md")
		assertSmokeFileContent(t, output, original)
		stdout, stderr := runSmokeCLI(t, executable, directory, cli.ExitSuccess,
			"input.pdf", "-o", "output.md", "--force")
		if stdout != "" || stderr != "" {
			t.Fatalf("forced conversion streams = %q, %q, want empty", stdout, stderr)
		}
		assertSmokeFileContent(t, output, want)
		assertSmokeFileContent(t, filepath.Join(directory, "input.pdf"), input)
		assertSmokeFiles(t, directory, "input.pdf", "output.md")
	})

	t.Run("conversion-failure-is-transactional", func(t *testing.T) {
		for _, output := range []string{"stdout", "new-file", "existing-file"} {
			t.Run(output, func(t *testing.T) {
				directory := t.TempDir()
				input := []byte("%PDF-1.4\nmalformed self-authored smoke input\n")
				writeSmokeFile(t, filepath.Join(directory, "input.pdf"), input, 0o600)
				args := []string{"input.pdf", "-o", "output.md"}
				files := []string{"input.pdf"}
				original := []byte("existing user output\r\n")
				switch output {
				case "stdout":
					args[2] = "-"
				case "existing-file":
					writeSmokeFile(t, filepath.Join(directory, "output.md"), original, 0o600)
					args = append(args, "--force")
					files = append(files, "output.md")
				}
				runSmokeCLI(t, executable, directory, cli.ExitConversion, args...)
				assertSmokeFileContent(t, filepath.Join(directory, "input.pdf"), input)
				if output == "existing-file" {
					assertSmokeFileContent(t, filepath.Join(directory, "output.md"), original)
				}
				assertSmokeFiles(t, directory, files...)
			})
		}
	})

	t.Run("argument-and-path-errors", func(t *testing.T) {
		input := readSmokeFile(t, filepath.Join(fixtures, "simple.pdf"))
		for _, test := range []struct {
			name string
			args []string
			code int
		}{
			{name: "missing-input", code: cli.ExitUsage},
			{name: "extra-input", args: []string{"input.pdf", "second.pdf"}, code: cli.ExitUsage},
			{name: "unknown-option", args: []string{"--unknown"}, code: cli.ExitUsage},
			{name: "missing-output-value", args: []string{"input.pdf", "-o"}, code: cli.ExitUsage},
			{name: "force-stdout", args: []string{"input.pdf", "-o", "-", "--force"}, code: cli.ExitUsage},
			{name: "missing-file", args: []string{"missing.pdf"}, code: cli.ExitInput},
			{name: "directory-input", args: []string{"."}, code: cli.ExitInput},
			{name: "unrecognized-input", args: []string{"not-pdf.txt"}, code: cli.ExitInput},
			{name: "missing-output-directory", args: []string{"input.pdf", "-o", filepath.Join("missing", "output.md")}, code: cli.ExitOutput},
			{name: "output-directory", args: []string{"input.pdf", "-o", "."}, code: cli.ExitOutput},
			{name: "same-input-output", args: []string{"input.pdf", "-o", "input.pdf", "--force"}, code: cli.ExitOutput},
		} {
			t.Run(test.name, func(t *testing.T) {
				directory := t.TempDir()
				writeSmokeFile(t, filepath.Join(directory, "input.pdf"), input, 0o600)
				writeSmokeFile(t, filepath.Join(directory, "not-pdf.txt"), []byte("not a PDF\n"), 0o600)
				runSmokeCLI(t, executable, directory, test.code, test.args...)
				assertSmokeFileContent(t, filepath.Join(directory, "input.pdf"), input)
				assertSmokeFiles(t, directory, "input.pdf", "not-pdf.txt")
			})
		}
	})
}

func TestExecutableVersions(t *testing.T) {
	root := smokeRepositoryRoot(t)
	for _, test := range []struct {
		name    string
		version string
		want    string
	}{
		{name: "default-development", want: "dev"},
		{name: "injected-development", version: "dev", want: "dev"},
		{name: "release", version: "v0.1.0", want: "v0.1.0"},
		{name: "prerelease", version: "v0.1.0-rc.1", want: "v0.1.0-rc.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "transmutemd")
			if runtime.GOOS == "windows" {
				executable += ".exe"
			}
			buildSmokeExecutable(t, root, executable, test.version)
			directory := t.TempDir()
			stdout, stderr := runSmokeCLI(t, executable, directory, cli.ExitSuccess, "--version")
			if want := "transmutemd " + test.want + "\n"; stdout != want || stderr != "" {
				t.Fatalf("version streams = %q, %q; want %q, empty stderr", stdout, stderr, want)
			}
			assertSmokeFiles(t, directory)
		})
	}
}

func smokeRepositoryRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func buildSmokeExecutable(t *testing.T, root, executable, version string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	args := []string{"build", "-trimpath", "-o", executable}
	if version != "" {
		args = append(args, "-ldflags", "-X github.com/Patrick-Q-Jensen/TransmuteMD/internal/cli.Version="+version)
	}
	args = append(args, ".")
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = filepath.Join(root, "cmd", "transmutemd")
	command.Env = append(os.Environ(),
		"CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build smoke executable: %v\n%s", err, output)
	}
}

func smokeExecutable(t *testing.T, root string) string {
	t.Helper()

	name := "transmutemd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable := filepath.Join(t.TempDir(), name)
	if supplied, ok := os.LookupEnv("TRANSMUTEMD_SMOKE_EXECUTABLE"); ok {
		if supplied == "" || !filepath.IsAbs(supplied) {
			t.Fatal("TRANSMUTEMD_SMOKE_EXECUTABLE must be an absolute executable path")
		}
		writeSmokeFile(t, executable, readSmokeFile(t, supplied), 0o755)
	} else {
		buildSmokeExecutable(t, root, executable, "")
	}
	info, err := buildinfo.ReadFile(executable)
	if err != nil {
		t.Fatalf("read executable build metadata: %v", err)
	}
	if info.Path != "github.com/Patrick-Q-Jensen/TransmuteMD/cmd/transmutemd" {
		t.Fatalf("executable package = %q, want the TransmuteMD CLI", info.Path)
	}
	for _, want := range []struct {
		key   string
		value string
	}{
		{key: "GOOS", value: runtime.GOOS},
		{key: "GOARCH", value: runtime.GOARCH},
		{key: "CGO_ENABLED", value: "0"},
		{key: "-trimpath", value: "true"},
	} {
		found := false
		for _, setting := range info.Settings {
			if setting.Key == want.key && setting.Value == want.value {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("executable metadata does not contain %s=%s", want.key, want.value)
		}
	}
	t.Logf("testing standalone executable on %s/%s with CGo disabled and trimmed paths",
		runtime.GOOS, runtime.GOARCH)
	return executable
}

func runSmokeCLI(t *testing.T, executable, directory string, wantCode int, args ...string) (string, string) {
	t.Helper()

	home, temporary, searchPath := t.TempDir(), t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = directory
	command.Env = []string{
		"PATH=" + searchPath,
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + temporary,
		"TMP=" + temporary,
		"TEMP=" + temporary,
	}
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			t.Fatal("SystemRoot is required to run the Windows smoke executable")
		}
		command.Env = append(command.Env, "SystemRoot="+systemRoot)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("CLI %q interrupted or timed out: %v; stderr = %q", args, ctx.Err(), stderr.String())
	}
	code := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("execute CLI %q: %v; stderr = %q", args, err, stderr.String())
		}
		code = exitError.ExitCode()
	}
	if code != wantCode {
		t.Fatalf("CLI %q exit code = %d, want %d; stdout = %q; stderr = %q",
			args, code, wantCode, stdout.String(), stderr.String())
	}
	if wantCode != cli.ExitSuccess && (stdout.Len() != 0 || stderr.Len() == 0) {
		t.Fatalf("failed CLI %q streams = %q, %q; want empty stdout and a stderr diagnostic",
			args, stdout.String(), stderr.String())
	}
	assertSmokeFiles(t, home)
	assertSmokeFiles(t, temporary)
	assertSmokeFiles(t, searchPath)
	return stdout.String(), stderr.String()
}

func readSmokeFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read smoke file %q: %v", path, err)
	}
	return data
}

func writeSmokeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write smoke file %q: %v", path, err)
	}
}

func assertSmokeFileContent(t *testing.T, path string, want []byte) {
	t.Helper()

	if got := readSmokeFile(t, path); !bytes.Equal(got, want) {
		t.Fatalf("smoke file %q content = %q, want %q", path, got, want)
	}
}

func assertSmokeFiles(t *testing.T, directory string, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read smoke directory %q: %v", directory, err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("smoke directory %q entries = %q, want %q", directory, got, want)
	}
}

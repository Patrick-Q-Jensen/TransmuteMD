package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	programName = "transmutemd"

	usageText = `Usage: transmutemd [options] <input>

Options:
  -o, --output <path>  Write Markdown to path; use - for standard output.
  -f, --force          Replace an existing output file.
  -h, --help           Show this help.
      --version        Show version information.
`
)

var errUsage = errors.New("invalid command usage")

type options struct {
	input       string
	output      string
	outputSet   bool
	force       bool
	showHelp    bool
	showVersion bool
}

func parseOptions(arguments []string) (options, error) {
	var parsed options
	var inputs []string
	optionsEnded := false

	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if optionsEnded {
			inputs = append(inputs, argument)
			continue
		}

		switch {
		case argument == "--":
			optionsEnded = true
		case argument == "-h" || argument == "--help":
			parsed.showHelp = true
		case argument == "--version":
			parsed.showVersion = true
		case argument == "-f" || argument == "--force":
			parsed.force = true
		case argument == "-o" || argument == "--output":
			if index+1 >= len(arguments) {
				return options{}, fmt.Errorf("%w: %s requires a path", errUsage, argument)
			}
			index++
			if err := parsed.setOutput(arguments[index]); err != nil {
				return options{}, err
			}
		case strings.HasPrefix(argument, "--output="):
			if err := parsed.setOutput(strings.TrimPrefix(argument, "--output=")); err != nil {
				return options{}, err
			}
		case strings.HasPrefix(argument, "-"):
			return options{}, fmt.Errorf("%w: unknown option %q", errUsage, argument)
		default:
			inputs = append(inputs, argument)
		}
	}

	if parsed.showHelp || parsed.showVersion {
		return parsed, nil
	}
	if len(inputs) != 1 {
		return options{}, fmt.Errorf(
			"%w: exactly one input path is required, got %d",
			errUsage,
			len(inputs),
		)
	}
	parsed.input = inputs[0]
	if parsed.force && parsed.outputSet && parsed.output == "-" {
		return options{}, fmt.Errorf("%w: --force cannot be used with standard output", errUsage)
	}
	if !parsed.outputSet {
		parsed.output = defaultOutputPath(parsed.input)
	}
	return parsed, nil
}

func (parsed *options) setOutput(path string) error {
	if parsed.outputSet {
		return fmt.Errorf("%w: output may be specified only once", errUsage)
	}
	if path == "" {
		return fmt.Errorf("%w: output path must not be empty", errUsage)
	}
	parsed.output = path
	parsed.outputSet = true
	return nil
}

func defaultOutputPath(input string) string {
	extension := filepath.Ext(input)
	if extension == "" {
		return input + ".md"
	}
	output := strings.TrimSuffix(input, extension) + ".md"
	if sameCleanPath(input, output) {
		return input + ".md"
	}
	return output
}

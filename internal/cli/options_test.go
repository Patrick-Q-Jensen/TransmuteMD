package cli

import (
	"reflect"
	"testing"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
		want      options
	}{
		{
			name:      "default output",
			arguments: []string{"report.pdf"},
			want:      options{input: "report.pdf", output: "report.md"},
		},
		{
			name:      "input without extension",
			arguments: []string{"report"},
			want:      options{input: "report", output: "report.md"},
		},
		{
			name:      "input already markdown",
			arguments: []string{"report.md"},
			want:      options{input: "report.md", output: "report.md.md"},
		},
		{
			name:      "long output after input",
			arguments: []string{"report.pdf", "--output", "converted.md"},
			want: options{
				input:     "report.pdf",
				output:    "converted.md",
				outputSet: true,
			},
		},
		{
			name:      "short output and force",
			arguments: []string{"-f", "-o", "converted.md", "report.pdf"},
			want: options{
				input:     "report.pdf",
				output:    "converted.md",
				outputSet: true,
				force:     true,
			},
		},
		{
			name:      "standard output",
			arguments: []string{"--output=-", "report.pdf"},
			want: options{
				input:     "report.pdf",
				output:    "-",
				outputSet: true,
			},
		},
		{
			name:      "option terminator",
			arguments: []string{"--", "-report.pdf"},
			want:      options{input: "-report.pdf", output: "-report.md"},
		},
		{
			name:      "help",
			arguments: []string{"--help"},
			want:      options{showHelp: true},
		},
		{
			name:      "version",
			arguments: []string{"--version"},
			want:      options{showVersion: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOptions(test.arguments)
			if err != nil {
				t.Fatalf("parseOptions() returned an unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseOptions() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseOptionsRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
	}{
		{name: "missing input"},
		{name: "multiple inputs", arguments: []string{"one.pdf", "two.pdf"}},
		{name: "unknown option", arguments: []string{"--unknown", "report.pdf"}},
		{name: "missing output value", arguments: []string{"report.pdf", "--output"}},
		{name: "empty output value", arguments: []string{"report.pdf", "--output="}},
		{
			name:      "duplicate output",
			arguments: []string{"report.pdf", "-o", "one.md", "--output", "two.md"},
		},
		{
			name:      "force with standard output",
			arguments: []string{"report.pdf", "--output", "-", "--force"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseOptions(test.arguments); err == nil {
				t.Fatal("parseOptions() returned nil error for invalid arguments")
			}
		})
	}
}

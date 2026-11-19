package security

import "testing"

func TestAnalyzeShellAggregatesNestedEffects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		command   string
		dangerous bool
		known     bool
	}{
		{name: "quoted text", command: `echo "rm -rf ./tmp"`, known: true},
		{name: "safe substitution", command: "echo $(pwd)", known: true},
		{name: "nested deletion", command: "echo $(rm -rf ./tmp)", dangerous: true, known: true},
		{name: "pipeline suffix", command: "cat input | rm -f output", dangerous: true, known: true},
		{name: "hermetic redirect", command: "printf ok > /dev/null", known: true},
		{name: "file redirect", command: "printf ok > output", dangerous: true, known: true},
		{name: "dynamic expansion", command: "cat $FILE", known: false},
		{name: "unsupported operation", command: "project-reader --report", known: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := AnalyzeShell(test.command, ShellPOSIX)
			if got.Dangerous != test.dangerous || got.Known != test.known {
				t.Fatalf("AnalyzeShell(%q) = %+v, want dangerous=%t known=%t", test.command, got, test.dangerous, test.known)
			}
		})
	}
}

func TestAnalyzeShellRejectsUnknownDialectsAndMalformedSyntax(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		dialect ShellDialect
		command string
	}{
		{name: "fish", dialect: ShellDialect("fish"), command: "echo (rm -rf ./tmp)"},
		{name: "powershell", dialect: ShellDialect("powershell"), command: "Remove-Item -Recurse tmp"},
		{name: "unterminated quote", dialect: ShellPOSIX, command: `echo "unterminated`},
		{name: "unterminated substitution", dialect: ShellBash, command: "echo $(pwd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := AnalyzeShell(test.command, test.dialect)
			if got.Known || got.Dangerous {
				t.Fatalf("AnalyzeShell(%q, %q) = %+v, want unknown", test.command, test.dialect, got)
			}
		})
	}
}

func TestAnalyzeShellSupportsDialectSpecificEntryPoints(t *testing.T) {
	t.Parallel()
	for _, dialect := range []ShellDialect{ShellPOSIX, ShellBash, ShellZsh} {
		got := AnalyzeShell("pwd && printf ok", dialect)
		if !got.Known || got.Dangerous {
			t.Fatalf("dialect %q produced %+v", dialect, got)
		}
	}
}

func TestAnalyzeArgvPreservesLiteralBoundaries(t *testing.T) {
	t.Parallel()
	const printf = "printf"
	for _, argv := range [][]string{
		{printf, "%s", "$(rm -rf ./tmp)"},
		{"echo", "&&", "rm", "-rf", "./tmp"},
	} {
		got := AnalyzeArgv(argv)
		if !got.Known || got.Dangerous {
			t.Fatalf("AnalyzeArgv(%q) = %+v, want known literal argv", argv, got)
		}
	}
}

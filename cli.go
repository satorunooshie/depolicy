package depolicy

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	ExitOK           = 0
	ExitPolicyError  = 1
	ExitConfigError  = 2
	ExitRuntimeError = 3
)

type CLI struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (c CLI) Run(args []string) int {
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}
	if len(args) == 0 {
		c.usage()
		return ExitRuntimeError
	}

	switch args[0] {
	case "validate":
		return c.runValidate(args[1:])
	case "check":
		return c.runCheck(args[1:])
	case "explain":
		return c.runExplain(args[1:])
	case "-h", "--help", "help":
		c.usage()
		return ExitOK
	default:
		fmt.Fprintf(c.Stderr, "depolicy: unknown command %q\n", args[0])
		c.usage()
		return ExitRuntimeError
	}
}

func (c CLI) runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	configPath := fs.String("config", "", "path to .depolicy.yaml")
	if err := fs.Parse(args); err != nil {
		return ExitRuntimeError
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "depolicy validate: unexpected positional arguments")
		return ExitRuntimeError
	}

	path, err := c.resolveConfigPath(*configPath)
	if err != nil {
		return exitForLoadError(c.Stderr, err)
	}
	if _, err := LoadProjectConfig(path); err != nil {
		return exitForLoadError(c.Stderr, err)
	}
	fmt.Fprintln(c.Stdout, "depolicy: configuration ok")
	return ExitOK
}

func (c CLI) runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	configPath := fs.String("config", "", "path to .depolicy.yaml")
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return ExitRuntimeError
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(c.Stderr, "depolicy check: unsupported format %q\n", *format)
		return ExitRuntimeError
	}

	path, err := c.resolveConfigPath(*configPath)
	if err != nil {
		return exitForLoadError(c.Stderr, err)
	}
	diagnostics, err := Check(CheckOptions{ConfigPath: path, Patterns: fs.Args()})
	if err != nil {
		return exitForLoadError(c.Stderr, err)
	}

	if *format == "json" {
		if err := WriteJSONDiagnostics(c.Stdout, diagnostics); err != nil {
			fmt.Fprintf(c.Stderr, "depolicy check: %v\n", err)
			return ExitRuntimeError
		}
	} else {
		WriteTextDiagnostics(c.Stdout, diagnostics)
	}

	if DiagnosticsHaveRuntimeErrors(diagnostics) {
		return ExitRuntimeError
	}
	if DiagnosticsHavePolicyErrors(diagnostics) {
		return ExitPolicyError
	}
	return ExitOK
}

func (c CLI) runExplain(args []string) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	configPath := fs.String("config", "", "path to .depolicy.yaml")
	sourceRaw := fs.String("package", "", "source package selector")
	targetRaw := fs.String("import", "", "target package selector")
	if err := fs.Parse(args); err != nil {
		return ExitRuntimeError
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "depolicy explain: unexpected positional arguments")
		return ExitRuntimeError
	}
	if *sourceRaw == "" || *targetRaw == "" {
		fmt.Fprintln(c.Stderr, "depolicy explain: --package and --import are required")
		return ExitRuntimeError
	}

	source, err := parseConcretePackageRef(*sourceRaw)
	if err != nil {
		fmt.Fprintf(c.Stderr, "depolicy explain: %v\n", err)
		return ExitRuntimeError
	}
	if source.Kind != PackageKindLocal {
		fmt.Fprintln(c.Stderr, "depolicy explain: --package must be a local: package")
		return ExitRuntimeError
	}
	target, err := parseConcretePackageRef(*targetRaw)
	if err != nil {
		fmt.Fprintf(c.Stderr, "depolicy explain: %v\n", err)
		return ExitRuntimeError
	}
	path, err := c.resolveConfigPath(*configPath)
	if err != nil {
		return exitForLoadError(c.Stderr, err)
	}
	compiled, err := LoadProjectConfig(path)
	if err != nil {
		return exitForLoadError(c.Stderr, err)
	}
	result, err := compiled.Decide(source, target)
	if err != nil {
		fmt.Fprintf(c.Stderr, "depolicy explain: %v\n", err)
		return ExitPolicyError
	}
	writeExplain(c.Stdout, result)
	return ExitOK
}

func (c CLI) resolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return FindConfigFromWorkingDir(".")
}

func (c CLI) usage() {
	fmt.Fprintln(c.Stderr, "usage:")
	fmt.Fprintln(c.Stderr, "  depolicy validate [--config .depolicy.yaml]")
	fmt.Fprintln(c.Stderr, "  depolicy check [--config .depolicy.yaml] [--format text|json] [packages...]")
	fmt.Fprintln(c.Stderr, "  depolicy explain --package local:path --import local:path [--config .depolicy.yaml]")
}

func exitForLoadError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "depolicy: %v\n", err)
	var cfgErr *ConfigError
	if errors.As(err, &cfgErr) {
		return ExitConfigError
	}
	return ExitRuntimeError
}

func writeExplain(w io.Writer, result DecisionResult) {
	fmt.Fprintf(w, "%s\n\n", result.Decision)
	fmt.Fprintf(w, "package:\n  %s\n\n", result.Source.String())
	fmt.Fprintf(w, "import:\n  %s\n\n", result.Target.String())
	fmt.Fprintf(w, "policy:\n  %s\n\n", result.PolicyID)
	if result.DefaultDecision {
		fmt.Fprintln(w, "decision:")
		fmt.Fprintln(w, "  imports.default")
		return
	}
	fmt.Fprintf(w, "rule:\n  %s\n\n", result.RuleID)
	fmt.Fprintf(w, "matched:\n  %s\n", result.MatchedSelector)
}

func IsVetToolInvocation(args []string) bool {
	for _, arg := range args {
		if arg == "-flags" || strings.HasPrefix(arg, "-V") || strings.HasSuffix(arg, ".cfg") {
			return true
		}
	}
	return false
}

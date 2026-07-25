package depolicy

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	CodeImportDenied     = "import-denied"
	CodeUncoveredPackage = "uncovered-package"
	CodeAmbiguousPolicy  = "ambiguous-policy"
	CodeInvalidConfig    = "invalid-config"
	CodePackageLoadError = "package-load-error"
)

type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Package  string `json:"package,omitempty"`
	Import   string `json:"import,omitempty"`
	Policy   string `json:"policy,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Selector string `json:"selector,omitempty"`

	SourceRef string `json:"-"`
	TargetRef string `json:"-"`
}

type JSONReport struct {
	Version     int          `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Summary     Summary      `json:"summary"`
}

type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

func WriteTextDiagnostics(w io.Writer, diagnostics []Diagnostic) {
	for i, diagnostic := range diagnostics {
		if i > 0 {
			fmt.Fprintln(w)
		}
		file := diagnostic.File
		if file == "" {
			file = "-"
		}
		line := diagnostic.Line
		if line == 0 {
			line = 1
		}
		column := diagnostic.Column
		if column == 0 {
			column = 1
		}
		fmt.Fprintf(w, "%s:%d:%d: %s\n", file, line, column, diagnostic.Message)
		if diagnostic.SourceRef != "" {
			fmt.Fprintf(w, "\n  package: %s\n", diagnostic.SourceRef)
		}
		if diagnostic.TargetRef != "" {
			fmt.Fprintf(w, "  import:  %s\n", diagnostic.TargetRef)
		}
		if diagnostic.Policy != "" {
			fmt.Fprintf(w, "  policy:  %s\n", diagnostic.Policy)
		}
		if diagnostic.Rule != "" {
			fmt.Fprintf(w, "  rule:    %s\n", diagnostic.Rule)
		}
		if diagnostic.Selector != "" {
			fmt.Fprintf(w, "  matched: %s\n", diagnostic.Selector)
		}
	}
}

func WriteJSONDiagnostics(w io.Writer, diagnostics []Diagnostic) error {
	report := JSONReport{
		Version:     ConfigVersion,
		Diagnostics: diagnostics,
		Summary:     summarizeDiagnostics(diagnostics),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func summarizeDiagnostics(diagnostics []Diagnostic) Summary {
	var summary Summary
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "warning" {
			summary.Warnings++
		} else {
			summary.Errors++
		}
	}
	return summary
}

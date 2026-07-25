package depolicy

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type CheckOptions struct {
	ConfigPath string
	Patterns   []string
}

func Check(options CheckOptions) ([]Diagnostic, error) {
	configPath := options.ConfigPath
	var err error
	if configPath == "" {
		configPath, err = FindConfigFromWorkingDir(".")
		if err != nil {
			return nil, err
		}
	}
	compiled, err := LoadProjectConfig(configPath)
	if err != nil {
		return nil, err
	}

	patterns := options.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	pkgs, err := loadPackages(compiled.Module, patterns)
	if err != nil {
		return nil, err
	}

	var diagnostics []Diagnostic
	for _, pkg := range pkgs {
		for _, loadErr := range pkg.Errors {
			pos := packagePosition(compiled.Module, pkg, token.NoPos)
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "error",
				Code:     CodePackageLoadError,
				Message:  loadErr.Msg,
				File:     pos.File,
				Line:     pos.Line,
				Column:   pos.Column,
				Package:  pkg.PkgPath,
			})
		}
	}
	if len(diagnostics) > 0 {
		return diagnostics, nil
	}

	for _, pkg := range pkgs {
		source := ClassifyImportPath(compiled.Module, pkg.PkgPath)
		if source.Kind != PackageKindLocal {
			continue
		}

		matches := compiled.FindPolicy(source)
		if len(matches) == 0 {
			pos := packagePosition(compiled.Module, pkg, token.NoPos)
			diagnostics = append(diagnostics, Diagnostic{
				Severity:  "error",
				Code:      CodeUncoveredPackage,
				Message:   fmt.Sprintf("package %q is not covered by any depolicy policy", pkg.PkgPath),
				File:      pos.File,
				Line:      pos.Line,
				Column:    pos.Column,
				Package:   pkg.PkgPath,
				SourceRef: source.String(),
			})
			continue
		}
		if len(matches) > 1 {
			pos := packagePosition(compiled.Module, pkg, token.NoPos)
			diagnostics = append(diagnostics, Diagnostic{
				Severity:  "error",
				Code:      CodeAmbiguousPolicy,
				Message:   ambiguousPolicyMessage(source, matches),
				File:      pos.File,
				Line:      pos.Line,
				Column:    pos.Column,
				Package:   pkg.PkgPath,
				SourceRef: source.String(),
			})
			continue
		}

		for _, file := range pkg.Syntax {
			for _, spec := range file.Imports {
				diagnostic, ok := checkImport(compiled, pkg, spec, source)
				if ok {
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}
	}
	return diagnostics, nil
}

func loadPackages(module ModuleInfo, patterns []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Dir: module.RootDir,
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax,
		Tests: false,
	}
	return packages.Load(cfg, patterns...)
}

func checkImport(compiled *CompiledConfig, pkg *packages.Package, spec *ast.ImportSpec, source PackageRef) (Diagnostic, bool) {
	importPath, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return Diagnostic{}, false
	}
	target := ClassifyImportPath(compiled.Module, importPath)
	result, err := compiled.Decide(source, target)
	if err != nil {
		return Diagnostic{}, false
	}
	if result.Decision != DecisionDeny {
		return Diagnostic{}, false
	}

	pos := packagePosition(compiled.Module, pkg, spec.Path.Pos())
	message := fmt.Sprintf("import %q is denied by policy %q", importPath, result.PolicyID)
	if result.RuleID != "" {
		message = fmt.Sprintf("import %q is denied by policy %q rule %q", importPath, result.PolicyID, result.RuleID)
	}
	return Diagnostic{
		Severity:  "error",
		Code:      CodeImportDenied,
		Message:   message,
		File:      pos.File,
		Line:      pos.Line,
		Column:    pos.Column,
		Package:   pkg.PkgPath,
		Import:    importPath,
		Policy:    result.PolicyID,
		Rule:      result.RuleID,
		Selector:  result.MatchedSelector,
		SourceRef: source.String(),
		TargetRef: target.String(),
	}, true
}

type sourcePosition struct {
	File   string
	Line   int
	Column int
}

func packagePosition(module ModuleInfo, pkg *packages.Package, pos token.Pos) sourcePosition {
	var position token.Position
	if pos.IsValid() && pkg.Fset != nil {
		position = pkg.Fset.PositionFor(pos, false)
	} else if len(pkg.Syntax) > 0 && pkg.Fset != nil {
		position = pkg.Fset.PositionFor(pkg.Syntax[0].Package, false)
	} else if len(pkg.GoFiles) > 0 {
		position = token.Position{Filename: pkg.GoFiles[0], Line: 1, Column: 1}
	}
	if position.Line == 0 {
		position.Line = 1
	}
	if position.Column == 0 {
		position.Column = 1
	}
	file := position.Filename
	if rel, err := filepath.Rel(module.RootDir, file); err == nil && !strings.HasPrefix(rel, "..") {
		file = rel
	}
	return sourcePosition{File: file, Line: position.Line, Column: position.Column}
}

func ambiguousPolicyMessage(source PackageRef, matches []PolicyMatch) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %q matches multiple policies:", source.String())
	for _, match := range matches {
		fmt.Fprintf(&b, "\n\n  policy %q:\n    %s", match.Policy.ID, match.Selector)
	}
	return b.String()
}

func DiagnosticsHavePolicyErrors(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case CodeImportDenied, CodeUncoveredPackage, CodeAmbiguousPolicy:
			return true
		}
	}
	return false
}

func DiagnosticsHaveRuntimeErrors(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == CodePackageLoadError {
			return true
		}
	}
	return false
}

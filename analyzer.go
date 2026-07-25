package depolicy

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name: "depolicy",
	Doc:  "depolicy reports imports that violate declarative dependency policies",
	Run:  runAnalyzer,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
}

func runAnalyzer(pass *analysis.Pass) (any, error) {
	var filenames []string
	for _, file := range pass.Files {
		filename := pass.Fset.PositionFor(file.Pos(), false).Filename
		if filename != "" {
			filenames = append(filenames, filename)
		}
	}
	configPath, err := FindConfigFromFiles(filenames...)
	if err != nil {
		return nil, err
	}
	compiled, err := loadProjectConfigForAnalyzer(configPath)
	if err != nil {
		return nil, err
	}

	source := ClassifyImportPath(compiled.Module, pass.Pkg.Path())
	if source.Kind != PackageKindLocal {
		return nil, nil
	}

	matches := compiled.FindPolicy(source)
	if len(matches) == 0 {
		pass.Report(analysis.Diagnostic{
			Pos:      packageReportPos(pass),
			Category: CodeUncoveredPackage,
			Message:  fmt.Sprintf("package %q is not covered by any depolicy policy", source.String()),
		})
		return nil, nil
	}
	if len(matches) > 1 {
		pass.Report(analysis.Diagnostic{
			Pos:      packageReportPos(pass),
			Category: CodeAmbiguousPolicy,
			Message:  ambiguousPolicyMessage(source, matches),
		})
		return nil, nil
	}

	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	inspect.Preorder([]ast.Node{(*ast.ImportSpec)(nil)}, func(n ast.Node) {
		spec := n.(*ast.ImportSpec)
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return
		}
		target := ClassifyImportPath(compiled.Module, importPath)
		result, err := compiled.Decide(source, target)
		if err != nil || result.Decision != DecisionDeny {
			return
		}
		message := fmt.Sprintf("import %q is denied by policy %q", importPath, result.PolicyID)
		if result.RuleID != "" {
			message = fmt.Sprintf("import %q is denied by policy %q rule %q", importPath, result.PolicyID, result.RuleID)
		}
		pass.Report(analysis.Diagnostic{
			Pos:      spec.Path.Pos(),
			Category: CodeImportDenied,
			Message:  message,
		})
	})
	return nil, nil
}

func packageReportPos(pass *analysis.Pass) token.Pos {
	if len(pass.Files) == 0 {
		return token.NoPos
	}
	return pass.Files[0].Package
}

var analyzerConfigCache sync.Map

type analyzerConfigCacheEntry struct {
	once   sync.Once
	config *CompiledConfig
	err    error
}

func loadProjectConfigForAnalyzer(configPath string) (*CompiledConfig, error) {
	value, _ := analyzerConfigCache.LoadOrStore(configPath, &analyzerConfigCacheEntry{})
	entry := value.(*analyzerConfigCacheEntry)
	entry.once.Do(func() {
		entry.config, entry.err = LoadProjectConfig(configPath)
	})
	return entry.config, entry.err
}

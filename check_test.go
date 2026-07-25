package depolicy

import (
	"sort"
	"strings"
	"testing"
)

func TestCheckProject(t *testing.T) {
	diagnostics, err := Check(CheckOptions{
		ConfigPath: "testdata/project/.depolicy.yaml",
		Patterns:   []string{"./..."},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]Diagnostic{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == CodePackageLoadError {
			t.Fatalf("unexpected package load error: %#v", diagnostic)
		}
		got[diagnosticKey(diagnostic)] = diagnostic
	}

	want := []string{
		"ambiguous-policy|local:ambiguous/service|||",
		"import-denied|local:api/main/handler|std:errors|api-handler|deny-errors",
		"import-denied|local:api/main/handler|local:component/billing/repository|api-handler|deny-components",
		"import-denied|local:api/main/handler|local:generated/sqlc|api-handler|deny-generated",
		"import-denied|local:api/main/handler|local:infra/database|api-handler|deny-infra",
		"import-denied|local:component/billing/service|local:api/main/handler|component|deny-api",
		"import-denied|local:component/billing/service|local:component/user/service|component|deny-other-components",
		"import-denied|local:core/logging|local:domain/user/entity|core|",
		"import-denied|local:domain/order/service|local:domain/user/entity|domain-context|deny-other-contexts",
		"import-denied|local:infra/repository|local:generated/sqlc|infra|deny-generated",
		"import-denied|local:integration/client|external:example.com/ext/forbidden|integration|deny-forbidden-external",
		"import-denied|local:usecase/order|std:errors|usecase|deny-errors",
		"import-denied|local:usecase/order|local:api/main/handler|usecase|deny-api",
		"import-denied|local:usecase/order|local:generated/sqlc|usecase|deny-generated",
		"uncovered-package|local:uncovered/service|||",
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing diagnostic %q\nall diagnostics:\n%s", key, formatDiagnosticKeys(got))
		}
	}
	if len(got) != len(want) {
		t.Fatalf("diagnostics = %d, want %d\nall diagnostics:\n%s", len(got), len(want), formatDiagnosticKeys(got))
	}
}

func diagnosticKey(d Diagnostic) string {
	return strings.Join([]string{
		d.Code,
		d.SourceRef,
		d.TargetRef,
		d.Policy,
		d.Rule,
	}, "|")
}

func formatDiagnosticKeys(diagnostics map[string]Diagnostic) string {
	keys := make([]string, 0, len(diagnostics))
	for key := range diagnostics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}

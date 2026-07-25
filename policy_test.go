package depolicy

import "testing"

func TestPolicyDecisionWithSameContext(t *testing.T) {
	cfg := mustConfig(t, `
version: 1
package-sets:
  domain-shared:
    - local:domain/shared/...
rule-sets: {}
policies:
  - id: domain-context
    packages:
      - local:domain/{context}/...
    imports:
      default: allow
      rules:
        - id: allow-same-context
          allow:
            - local:domain/{context}/...
        - id: allow-domain-shared
          allow:
            - set:domain-shared
        - id: deny-other-contexts
          deny:
            - local:domain/...
`)
	compiled, err := CompileConfig(cfg, ModuleInfo{Path: "github.com/example/backend", RootDir: "."})
	if err != nil {
		t.Fatal(err)
	}

	result, err := compiled.Decide(
		PackageRef{Kind: PackageKindLocal, Path: "domain/order/service"},
		PackageRef{Kind: PackageKindLocal, Path: "domain/order/entity"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionAllow || result.RuleID != "allow-same-context" {
		t.Fatalf("same context decision = %#v", result)
	}

	result, err = compiled.Decide(
		PackageRef{Kind: PackageKindLocal, Path: "domain/order/service"},
		PackageRef{Kind: PackageKindLocal, Path: "domain/user/entity"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDeny || result.RuleID != "deny-other-contexts" {
		t.Fatalf("other context decision = %#v", result)
	}
}

func TestCompileRejectsUndefinedVariable(t *testing.T) {
	cfg := mustConfig(t, `
version: 1
policies:
  - id: api
    packages:
      - local:api/...
    imports:
      default: allow
      rules:
        - id: allow-domain
          allow:
            - local:domain/{context}/...
`)
	_, err := CompileConfig(cfg, ModuleInfo{Path: "github.com/example/backend", RootDir: "."})
	if err == nil {
		t.Fatal("expected undefined variable error")
	}
}

func mustConfig(t *testing.T, raw string) *Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(raw), "test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

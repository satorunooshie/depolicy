package depolicy

import "testing"

func TestSelectorMatchesVariables(t *testing.T) {
	selector, err := ParseSelector("local:domain/{context}/...")
	if err != nil {
		t.Fatal(err)
	}
	bindings, ok := selector.Match(PackageRef{Kind: PackageKindLocal, Path: "domain/order/service"}, nil, true)
	if !ok {
		t.Fatal("selector did not match")
	}
	if bindings["context"] != "order" {
		t.Fatalf("context = %q, want order", bindings["context"])
	}

	target, err := ParseSelector("local:domain/{context}/...")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := target.Match(PackageRef{Kind: PackageKindLocal, Path: "domain/order/entity"}, bindings, false); !ok {
		t.Fatal("target selector should match same context")
	}
	if _, ok := target.Match(PackageRef{Kind: PackageKindLocal, Path: "domain/user/entity"}, bindings, false); ok {
		t.Fatal("target selector should not match another context")
	}
}

func TestParseSelectorRejectsInvalidRest(t *testing.T) {
	if _, err := ParseSelector("local:domain/.../entity"); err == nil {
		t.Fatal("expected error")
	}
}

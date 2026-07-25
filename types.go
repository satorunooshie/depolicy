package depolicy

import "fmt"

const (
	ConfigFileName = ".depolicy.yaml"
	ConfigVersion  = 1
)

type Decision string

const (
	DecisionAllow Decision = "ALLOW"
	DecisionDeny  Decision = "DENY"
)

type PackageKind string

const (
	PackageKindStd      PackageKind = "std"
	PackageKindLocal    PackageKind = "local"
	PackageKindExternal PackageKind = "external"
	PackageKindSet      PackageKind = "set"
)

func (k PackageKind) Prefix() string {
	return string(k) + ":"
}

type PackageRef struct {
	Kind       PackageKind
	Path       string
	ImportPath string
}

func (p PackageRef) String() string {
	return p.Kind.Prefix() + p.Path
}

type ModuleInfo struct {
	Path    string
	RootDir string
	GoMod   string
}

type Config struct {
	Path        string
	Version     int
	PackageSets map[string][]string
	RuleSets    map[string][]RawRule
	Policies    []RawPolicy
}

type RawPolicy struct {
	ID       string
	Message  string
	Packages []string
	Imports  RawImports
}

type RawImports struct {
	Default string
	Rules   []RawRule
}

type RawRule struct {
	ID      string
	Use     string
	Allow   []string
	Deny    []string
	Message string
}

type CompiledConfig struct {
	ConfigPath  string
	Module      ModuleInfo
	PackageSets map[string][]Selector
	Policies    []Policy
}

type Policy struct {
	ID               string
	Message          string
	PackageSelectors []Selector
	Default          Decision
	Rules            []Rule
}

type Rule struct {
	ID        string
	Message   string
	Decision  Decision
	Selectors []TargetSelector
}

type TargetSelector struct {
	Display      string
	Alternatives []Selector
}

type PolicyMatch struct {
	Policy        *Policy
	Selector      string
	Bindings      map[string]string
	MatchedPolicy int
}

type DecisionResult struct {
	Decision        Decision
	Source          PackageRef
	Target          PackageRef
	PolicyID        string
	RuleID          string
	MatchedSelector string
	DefaultDecision bool
	Message         string
}

type AssignmentKind string

const (
	AssignmentUncovered AssignmentKind = "uncovered"
	AssignmentAmbiguous AssignmentKind = "ambiguous"
)

type AssignmentError struct {
	Kind    AssignmentKind
	Source  PackageRef
	Matches []PolicyMatch
}

func (e *AssignmentError) Error() string {
	switch e.Kind {
	case AssignmentUncovered:
		return fmt.Sprintf("package %q is not covered by any policy", e.Source.String())
	case AssignmentAmbiguous:
		return fmt.Sprintf("package %q matches multiple policies", e.Source.String())
	default:
		return fmt.Sprintf("package %q has invalid policy assignment", e.Source.String())
	}
}

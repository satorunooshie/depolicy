package depolicy

import (
	"fmt"
	"strings"
)

func CompileConfig(cfg *Config, module ModuleInfo) (*CompiledConfig, error) {
	compiler := &configCompiler{
		cfg:        cfg,
		module:     module,
		setCache:   map[string][]Selector{},
		ruleCache:  map[string][]RawRule{},
		ruleActive: map[string]bool{},
		setActive:  map[string]bool{},
	}
	return compiler.compile()
}

type configCompiler struct {
	cfg        *Config
	module     ModuleInfo
	setCache   map[string][]Selector
	setActive  map[string]bool
	ruleCache  map[string][]RawRule
	ruleActive map[string]bool
}

func (c *configCompiler) compile() (*CompiledConfig, error) {
	for name := range c.cfg.PackageSets {
		if _, err := c.resolvePackageSet(name); err != nil {
			return nil, err
		}
	}
	for name := range c.cfg.RuleSets {
		if _, err := c.expandRuleSet(name); err != nil {
			return nil, err
		}
	}

	compiled := &CompiledConfig{
		ConfigPath:  c.cfg.Path,
		Module:      c.module,
		PackageSets: c.setCache,
	}
	for _, rawPolicy := range c.cfg.Policies {
		policy, err := c.compilePolicy(rawPolicy)
		if err != nil {
			return nil, err
		}
		compiled.Policies = append(compiled.Policies, policy)
	}
	return compiled, nil
}

func (c *configCompiler) resolvePackageSet(name string) ([]Selector, error) {
	if selectors, ok := c.setCache[name]; ok {
		return selectors, nil
	}
	rawSelectors, ok := c.cfg.PackageSets[name]
	if !ok {
		return nil, fmt.Errorf("undefined package set %q", name)
	}
	if c.setActive[name] {
		return nil, fmt.Errorf("package set %q has a circular reference", name)
	}
	c.setActive[name] = true
	defer delete(c.setActive, name)

	var selectors []Selector
	for _, raw := range rawSelectors {
		selector, err := ParseSelector(raw)
		if err != nil {
			return nil, err
		}
		if selector.Kind == PackageKindSet {
			nested, err := c.resolvePackageSet(selector.SetName)
			if err != nil {
				return nil, err
			}
			selectors = append(selectors, nested...)
			continue
		}
		if len(selector.Vars) > 0 {
			return nil, fmt.Errorf("package set %q must not use path variables in selector %q", name, raw)
		}
		selectors = append(selectors, selector)
	}
	c.setCache[name] = selectors
	return selectors, nil
}

func (c *configCompiler) expandRuleSet(name string) ([]RawRule, error) {
	if rules, ok := c.ruleCache[name]; ok {
		return rules, nil
	}
	rawRules, ok := c.cfg.RuleSets[name]
	if !ok {
		return nil, fmt.Errorf("undefined rule set %q", name)
	}
	if c.ruleActive[name] {
		return nil, fmt.Errorf("rule set %q has a circular reference", name)
	}
	c.ruleActive[name] = true
	defer delete(c.ruleActive, name)

	var rules []RawRule
	for _, rawRule := range rawRules {
		if rawRule.Use != "" {
			nested, err := c.expandRuleSet(rawRule.Use)
			if err != nil {
				return nil, err
			}
			rules = append(rules, nested...)
			continue
		}
		for _, rawSelector := range append(append([]string{}, rawRule.Allow...), rawRule.Deny...) {
			selector, err := ParseSelector(rawSelector)
			if err != nil {
				return nil, err
			}
			if len(selector.Vars) > 0 {
				return nil, fmt.Errorf("rule set %q must not use path variables in selector %q", name, rawSelector)
			}
			if selector.Kind == PackageKindSet {
				if _, err := c.resolvePackageSet(selector.SetName); err != nil {
					return nil, err
				}
			}
		}
		rules = append(rules, rawRule)
	}
	if err := detectDuplicateRuleIDs(rules, "rule set "+name); err != nil {
		return nil, err
	}
	c.ruleCache[name] = rules
	return rules, nil
}

func (c *configCompiler) compilePolicy(raw RawPolicy) (Policy, error) {
	policy := Policy{
		ID:      raw.ID,
		Message: raw.Message,
		Default: parseDecision(raw.Imports.Default),
	}
	policyVars := map[string]struct{}{}
	for _, rawSelector := range raw.Packages {
		selector, err := ParseSelector(rawSelector)
		if err != nil {
			return Policy{}, err
		}
		if selector.Kind != PackageKindLocal {
			return Policy{}, fmt.Errorf("policy %q packages must use local: selectors, got %q", raw.ID, rawSelector)
		}
		for name := range selector.Vars {
			policyVars[name] = struct{}{}
		}
		policy.PackageSelectors = append(policy.PackageSelectors, selector)
	}

	expandedRules, err := c.expandPolicyRules(raw.Imports.Rules)
	if err != nil {
		return Policy{}, err
	}
	if err := detectDuplicateRuleIDs(expandedRules, "policy "+raw.ID); err != nil {
		return Policy{}, err
	}
	for _, rawRule := range expandedRules {
		rule, err := c.compileRule(raw.ID, rawRule, policyVars)
		if err != nil {
			return Policy{}, err
		}
		policy.Rules = append(policy.Rules, rule)
	}
	return policy, nil
}

func (c *configCompiler) expandPolicyRules(rawRules []RawRule) ([]RawRule, error) {
	var rules []RawRule
	for _, rawRule := range rawRules {
		if rawRule.Use == "" {
			rules = append(rules, rawRule)
			continue
		}
		expanded, err := c.expandRuleSet(rawRule.Use)
		if err != nil {
			return nil, err
		}
		rules = append(rules, expanded...)
	}
	return rules, nil
}

func (c *configCompiler) compileRule(policyID string, raw RawRule, policyVars map[string]struct{}) (Rule, error) {
	rule := Rule{ID: raw.ID, Message: raw.Message}
	var rawSelectors []string
	if len(raw.Allow) > 0 {
		rule.Decision = DecisionAllow
		rawSelectors = raw.Allow
	} else {
		rule.Decision = DecisionDeny
		rawSelectors = raw.Deny
	}
	for _, rawSelector := range rawSelectors {
		target, err := c.compileTargetSelector(policyID, rawSelector, policyVars)
		if err != nil {
			return Rule{}, err
		}
		rule.Selectors = append(rule.Selectors, target)
	}
	return rule, nil
}

func (c *configCompiler) compileTargetSelector(policyID, rawSelector string, policyVars map[string]struct{}) (TargetSelector, error) {
	selector, err := ParseSelector(rawSelector)
	if err != nil {
		return TargetSelector{}, err
	}
	if selector.Kind == PackageKindSet {
		alternatives, err := c.resolvePackageSet(selector.SetName)
		if err != nil {
			return TargetSelector{}, err
		}
		return TargetSelector{Display: rawSelector, Alternatives: alternatives}, nil
	}
	for name := range selector.Vars {
		if _, ok := policyVars[name]; !ok {
			return TargetSelector{}, fmt.Errorf("policy %q selector %q references undefined path variable %q", policyID, rawSelector, name)
		}
	}
	return TargetSelector{Display: rawSelector, Alternatives: []Selector{selector}}, nil
}

func detectDuplicateRuleIDs(rules []RawRule, scope string) error {
	seen := map[string]struct{}{}
	for _, rule := range rules {
		if rule.Use != "" {
			continue
		}
		if _, ok := seen[rule.ID]; ok {
			return fmt.Errorf("%s contains duplicate rule id %q after rule set expansion", scope, rule.ID)
		}
		seen[rule.ID] = struct{}{}
	}
	return nil
}

func parseDecision(raw string) Decision {
	switch strings.ToLower(raw) {
	case "allow":
		return DecisionAllow
	default:
		return DecisionDeny
	}
}

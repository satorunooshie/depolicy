package depolicy

func (c *CompiledConfig) FindPolicy(source PackageRef) []PolicyMatch {
	if source.Kind != PackageKindLocal {
		return nil
	}
	var matches []PolicyMatch
	for i := range c.Policies {
		policy := &c.Policies[i]
		for _, selector := range policy.PackageSelectors {
			bindings, ok := selector.Match(source, nil, true)
			if !ok {
				continue
			}
			matches = append(matches, PolicyMatch{
				Policy:        policy,
				Selector:      selector.Raw,
				Bindings:      bindings,
				MatchedPolicy: i,
			})
			break
		}
	}
	return matches
}

func (c *CompiledConfig) Decide(source, target PackageRef) (DecisionResult, error) {
	matches := c.FindPolicy(source)
	if len(matches) == 0 {
		return DecisionResult{}, &AssignmentError{Kind: AssignmentUncovered, Source: source}
	}
	if len(matches) > 1 {
		return DecisionResult{}, &AssignmentError{Kind: AssignmentAmbiguous, Source: source, Matches: matches}
	}
	match := matches[0]
	policy := match.Policy

	for _, rule := range policy.Rules {
		for _, targetSelector := range rule.Selectors {
			if targetSelector.Matches(target, match.Bindings) {
				return DecisionResult{
					Decision:        rule.Decision,
					Source:          source,
					Target:          target,
					PolicyID:        policy.ID,
					RuleID:          rule.ID,
					MatchedSelector: targetSelector.Display,
					DefaultDecision: false,
					Message:         rule.Message,
				}, nil
			}
		}
	}

	return DecisionResult{
		Decision:        policy.Default,
		Source:          source,
		Target:          target,
		PolicyID:        policy.ID,
		DefaultDecision: true,
		Message:         policy.Message,
	}, nil
}

func (s TargetSelector) Matches(target PackageRef, bindings map[string]string) bool {
	for _, alternative := range s.Alternatives {
		if _, ok := alternative.Match(target, bindings, false); ok {
			return true
		}
	}
	return false
}

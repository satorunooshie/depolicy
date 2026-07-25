package depolicy

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
)

type segmentKind int

const (
	segmentExact segmentKind = iota
	segmentStar
	segmentVariable
	segmentRest
)

type selectorSegment struct {
	kind  segmentKind
	value string
}

type Selector struct {
	Raw      string
	Kind     PackageKind
	SetName  string
	Segments []selectorSegment
	Vars     map[string]struct{}
}

var variableNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ParseSelector(raw string) (Selector, error) {
	if raw == "" {
		return Selector{}, fmt.Errorf("selector must not be empty")
	}

	kind, body, ok := splitSelectorPrefix(raw)
	if !ok {
		return Selector{}, fmt.Errorf("selector %q must start with std:, local:, external:, or set:", raw)
	}

	if kind == PackageKindSet {
		if body == "" {
			return Selector{}, fmt.Errorf("selector %q must name a package set", raw)
		}
		if strings.ContainsAny(body, "/*{}") {
			return Selector{}, fmt.Errorf("set selector %q must be a concrete set name", raw)
		}
		return Selector{Raw: raw, Kind: kind, SetName: body}, nil
	}

	if body == "" && kind != PackageKindLocal {
		return Selector{}, fmt.Errorf("selector %q must include a package path", raw)
	}
	if body == "..." {
		return Selector{
			Raw:      raw,
			Kind:     kind,
			Segments: []selectorSegment{{kind: segmentRest}},
		}, nil
	}

	var segments []selectorSegment
	vars := map[string]struct{}{}
	if body != "" {
		parts := strings.Split(body, "/")
		for i, part := range parts {
			if part == "" {
				return Selector{}, fmt.Errorf("selector %q contains an empty path segment", raw)
			}
			switch {
			case part == "...":
				if i != len(parts)-1 {
					return Selector{}, fmt.Errorf("selector %q uses ... before the final segment", raw)
				}
				segments = append(segments, selectorSegment{kind: segmentRest})
			case strings.Contains(part, "..."):
				return Selector{}, fmt.Errorf("selector %q uses ... outside a whole final segment", raw)
			case part == "*":
				segments = append(segments, selectorSegment{kind: segmentStar})
			case strings.HasPrefix(part, "{") || strings.HasSuffix(part, "}"):
				if !strings.HasPrefix(part, "{") || !strings.HasSuffix(part, "}") || len(part) < 3 {
					return Selector{}, fmt.Errorf("selector %q has an invalid path variable", raw)
				}
				name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
				if !variableNameRE.MatchString(name) {
					return Selector{}, fmt.Errorf("selector %q has invalid path variable name %q", raw, name)
				}
				segments = append(segments, selectorSegment{kind: segmentVariable, value: name})
				vars[name] = struct{}{}
			case strings.ContainsAny(part, "*{}"):
				return Selector{}, fmt.Errorf("selector %q has a wildcard or variable inside a path segment", raw)
			default:
				segments = append(segments, selectorSegment{kind: segmentExact, value: part})
			}
		}
	}

	return Selector{Raw: raw, Kind: kind, Segments: segments, Vars: vars}, nil
}

func splitSelectorPrefix(raw string) (PackageKind, string, bool) {
	prefixes := []PackageKind{
		PackageKindStd,
		PackageKindLocal,
		PackageKindExternal,
		PackageKindSet,
	}
	for _, kind := range prefixes {
		prefix := kind.Prefix()
		if after, ok := strings.CutPrefix(raw, prefix); ok {
			return kind, after, true
		}
	}
	return "", "", false
}

func (s Selector) Match(pkg PackageRef, bindings map[string]string, captureVariables bool) (map[string]string, bool) {
	if s.Kind != pkg.Kind {
		return nil, false
	}
	if s.Kind == PackageKindSet {
		return nil, false
	}

	next := cloneBindings(bindings)
	parts := splitPackagePath(pkg.Path)

	for i, segment := range s.Segments {
		if segment.kind == segmentRest {
			if i != len(s.Segments)-1 {
				return nil, false
			}
			return next, true
		}
		if len(parts) == 0 {
			return nil, false
		}

		current := parts[0]
		parts = parts[1:]

		switch segment.kind {
		case segmentExact:
			if current != segment.value {
				return nil, false
			}
		case segmentStar:
		case segmentVariable:
			existing, ok := next[segment.value]
			if ok {
				if existing != current {
					return nil, false
				}
				continue
			}
			if !captureVariables {
				return nil, false
			}
			next[segment.value] = current
		default:
			return nil, false
		}
	}

	if len(parts) != 0 {
		return nil, false
	}
	return next, true
}

func (s Selector) IsConcretePackageSelector() bool {
	if s.Kind == PackageKindSet {
		return false
	}
	for _, segment := range s.Segments {
		if segment.kind != segmentExact {
			return false
		}
	}
	return true
}

func splitPackagePath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func cloneBindings(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func parseConcretePackageRef(raw string) (PackageRef, error) {
	selector, err := ParseSelector(raw)
	if err != nil {
		return PackageRef{}, err
	}
	if selector.Kind == PackageKindSet {
		return PackageRef{}, fmt.Errorf("concrete package must not use set: selector %q", raw)
	}
	if !selector.IsConcretePackageSelector() {
		return PackageRef{}, fmt.Errorf("concrete package must not use wildcards or variables: %q", raw)
	}
	return PackageRef{Kind: selector.Kind, Path: strings.TrimPrefix(raw, selector.Kind.Prefix())}, nil
}

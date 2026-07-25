package depolicy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type ConfigError struct {
	Path    string
	Line    int
	Column  int
	Message string
}

func (e *ConfigError) Error() string {
	loc := e.Path
	if e.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", loc, e.Line, e.Column)
	}
	if loc == "" {
		return e.Message
	}
	return loc + ": " + e.Message
}

func newConfigError(path string, node *yaml.Node, format string, args ...any) *ConfigError {
	err := &ConfigError{
		Path:    path,
		Message: fmt.Sprintf(format, args...),
	}
	if node != nil {
		err.Line = node.Line
		err.Column = node.Column
	}
	return err
}

func FindConfigFromWorkingDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		path := filepath.Join(abs, ConfigFileName)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", &ConfigError{Message: fmt.Sprintf("%s was not found", ConfigFileName)}
		}
		abs = parent
	}
}

func FindConfigFromFiles(filenames ...string) (string, error) {
	type candidate struct {
		path  string
		depth int
	}

	var candidates []candidate
	seen := map[string]struct{}{}
	for _, filename := range filenames {
		if filename == "" {
			continue
		}
		dir, err := filepath.Abs(filepath.Dir(filename))
		if err != nil {
			return "", err
		}
		for depth := 0; ; depth++ {
			path := filepath.Join(dir, ConfigFileName)
			if _, err := os.Stat(path); err == nil {
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					candidates = append(candidates, candidate{path: path, depth: depth})
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if len(candidates) == 0 {
		return "", &ConfigError{Message: fmt.Sprintf("%s was not found", ConfigFileName)}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth < candidates[j].depth
		}
		return candidates[i].path < candidates[j].path
	})
	return candidates[0].path, nil
}

func LoadConfig(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return ParseConfig(data, abs)
}

func ParseConfig(data []byte, path string) (*Config, error) {
	root, err := decodeStrictYAML(data, path)
	if err != nil {
		return nil, err
	}
	if root.Kind != yaml.MappingNode {
		return nil, newConfigError(path, root, "configuration root must be a mapping")
	}

	allowedTop := map[string]struct{}{
		"version":      {},
		"package-sets": {},
		"rule-sets":    {},
		"policies":     {},
	}
	fields := mappingFields(root)
	for key, node := range fields {
		if _, ok := allowedTop[key]; !ok {
			return nil, newConfigError(path, node.key, "unknown top-level field %q", key)
		}
		_ = node
	}

	versionNode, ok := fields["version"]
	if !ok {
		return nil, newConfigError(path, root, "missing required field %q", "version")
	}
	version, err := readInt(path, versionNode.value, "version")
	if err != nil {
		return nil, err
	}
	if version != ConfigVersion {
		return nil, newConfigError(path, versionNode.value, "unsupported config version %d", version)
	}

	policiesNode, ok := fields["policies"]
	if !ok {
		return nil, newConfigError(path, root, "missing required field %q", "policies")
	}

	cfg := &Config{
		Path:        path,
		Version:     version,
		PackageSets: map[string][]string{},
		RuleSets:    map[string][]RawRule{},
	}
	if node, ok := fields["package-sets"]; ok {
		cfg.PackageSets, err = readPackageSets(path, node.value)
		if err != nil {
			return nil, err
		}
	}
	if node, ok := fields["rule-sets"]; ok {
		cfg.RuleSets, err = readRuleSets(path, node.value)
		if err != nil {
			return nil, err
		}
	}
	cfg.Policies, err = readPolicies(path, policiesNode.value)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func decodeStrictYAML(data []byte, path string) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &ConfigError{Path: path, Message: "configuration is empty"}
		}
		return nil, &ConfigError{Path: path, Message: err.Error()}
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, &ConfigError{Path: path, Message: "multiple YAML documents are not supported"}
	} else if !errors.Is(err, io.EOF) {
		return nil, &ConfigError{Path: path, Message: err.Error()}
	}
	if len(doc.Content) == 0 {
		return nil, &ConfigError{Path: path, Message: "configuration is empty"}
	}
	root := doc.Content[0]
	if err := validateStrictYAMLNode(path, root); err != nil {
		return nil, err
	}
	return root, nil
}

func validateStrictYAMLNode(path string, node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Anchor != "" {
		return newConfigError(path, node, "YAML anchors are not supported")
	}
	allowedTags := map[string]struct{}{
		"!!map":  {},
		"!!seq":  {},
		"!!str":  {},
		"!!bool": {},
		"!!int":  {},
		"!!null": {},
	}
	if _, ok := allowedTags[node.Tag]; !ok {
		return newConfigError(path, node, "YAML tag %q is not supported", node.Tag)
	}

	switch node.Kind {
	case yaml.AliasNode:
		return newConfigError(path, node, "YAML aliases are not supported")
	case yaml.MappingNode:
		seen := map[string]*yaml.Node{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Value == "<<" || key.Tag == "!!merge" {
				return newConfigError(path, key, "YAML merge keys are not supported")
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return newConfigError(path, key, "mapping keys must be strings")
			}
			if previous, ok := seen[key.Value]; ok {
				return newConfigError(path, key, "duplicate key %q previously defined at line %d", key.Value, previous.Line)
			}
			seen[key.Value] = key
			if err := validateStrictYAMLNode(path, value); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := validateStrictYAMLNode(path, child); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
	default:
		return newConfigError(path, node, "unsupported YAML node kind")
	}
	return nil
}

type mappingField struct {
	key   *yaml.Node
	value *yaml.Node
}

func mappingFields(node *yaml.Node) map[string]mappingField {
	out := make(map[string]mappingField, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		out[key.Value] = mappingField{key: key, value: node.Content[i+1]}
	}
	return out
}

func readPackageSets(path string, node *yaml.Node) (map[string][]string, error) {
	if node.Kind != yaml.MappingNode {
		return nil, newConfigError(path, node, "package-sets must be a mapping")
	}
	out := map[string][]string{}
	for name, field := range mappingFields(node) {
		if name == "" {
			return nil, newConfigError(path, field.key, "package set name must not be empty")
		}
		selectors, err := readStringSequence(path, field.value, "package set "+name)
		if err != nil {
			return nil, err
		}
		if len(selectors) == 0 {
			return nil, newConfigError(path, field.value, "package set %q must not be empty", name)
		}
		out[name] = selectors
	}
	return out, nil
}

func readRuleSets(path string, node *yaml.Node) (map[string][]RawRule, error) {
	if node.Kind != yaml.MappingNode {
		return nil, newConfigError(path, node, "rule-sets must be a mapping")
	}
	out := map[string][]RawRule{}
	for name, field := range mappingFields(node) {
		if name == "" {
			return nil, newConfigError(path, field.key, "rule set name must not be empty")
		}
		rules, err := readRules(path, field.value)
		if err != nil {
			return nil, err
		}
		if len(rules) == 0 {
			return nil, newConfigError(path, field.value, "rule set %q must not be empty", name)
		}
		out[name] = rules
	}
	return out, nil
}

func readPolicies(path string, node *yaml.Node) ([]RawPolicy, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, newConfigError(path, node, "policies must be a sequence")
	}
	var policies []RawPolicy
	seenIDs := map[string]struct{}{}
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, newConfigError(path, item, "policy must be a mapping")
		}
		fields := mappingFields(item)
		for key, field := range fields {
			switch key {
			case "id", "packages", "imports", "message":
			default:
				return nil, newConfigError(path, field.key, "unknown policy field %q", key)
			}
		}
		idField, ok := fields["id"]
		if !ok {
			return nil, newConfigError(path, item, "policy is missing required field %q", "id")
		}
		id, err := readString(path, idField.value, "policy id")
		if err != nil {
			return nil, err
		}
		if id == "" {
			return nil, newConfigError(path, idField.value, "policy id must not be empty")
		}
		if _, ok := seenIDs[id]; ok {
			return nil, newConfigError(path, idField.value, "duplicate policy id %q", id)
		}
		seenIDs[id] = struct{}{}

		packagesField, ok := fields["packages"]
		if !ok {
			return nil, newConfigError(path, item, "policy %q is missing required field %q", id, "packages")
		}
		packages, err := readStringSequence(path, packagesField.value, "policy packages")
		if err != nil {
			return nil, err
		}
		if len(packages) == 0 {
			return nil, newConfigError(path, packagesField.value, "policy %q packages must not be empty", id)
		}
		seenPackageSelectors := map[string]struct{}{}
		for _, selector := range packages {
			if _, ok := seenPackageSelectors[selector]; ok {
				return nil, newConfigError(path, packagesField.value, "policy %q contains duplicate package selector %q", id, selector)
			}
			seenPackageSelectors[selector] = struct{}{}
		}

		importsField, ok := fields["imports"]
		if !ok {
			return nil, newConfigError(path, item, "policy %q is missing required field %q", id, "imports")
		}
		imports, err := readImports(path, importsField.value, id)
		if err != nil {
			return nil, err
		}
		message := ""
		if field, ok := fields["message"]; ok {
			message, err = readString(path, field.value, "policy message")
			if err != nil {
				return nil, err
			}
		}
		policies = append(policies, RawPolicy{
			ID:       id,
			Message:  message,
			Packages: packages,
			Imports:  imports,
		})
	}
	return policies, nil
}

func readImports(path string, node *yaml.Node, policyID string) (RawImports, error) {
	if node.Kind != yaml.MappingNode {
		return RawImports{}, newConfigError(path, node, "policy %q imports must be a mapping", policyID)
	}
	fields := mappingFields(node)
	for key, field := range fields {
		switch key {
		case "default", "rules":
		default:
			return RawImports{}, newConfigError(path, field.key, "unknown imports field %q", key)
		}
	}
	defaultField, ok := fields["default"]
	if !ok {
		return RawImports{}, newConfigError(path, node, "policy %q imports is missing required field %q", policyID, "default")
	}
	defaultValue, err := readString(path, defaultField.value, "imports.default")
	if err != nil {
		return RawImports{}, err
	}
	if defaultValue != "allow" && defaultValue != "deny" {
		return RawImports{}, newConfigError(path, defaultField.value, "imports.default must be allow or deny")
	}
	var rules []RawRule
	if rulesField, ok := fields["rules"]; ok {
		rules, err = readRules(path, rulesField.value)
		if err != nil {
			return RawImports{}, err
		}
	}
	return RawImports{Default: defaultValue, Rules: rules}, nil
}

func readRules(path string, node *yaml.Node) ([]RawRule, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, newConfigError(path, node, "rules must be a sequence")
	}
	var rules []RawRule
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, newConfigError(path, item, "rule must be a mapping")
		}
		fields := mappingFields(item)
		for key, field := range fields {
			switch key {
			case "id", "use", "allow", "deny", "message":
			default:
				return nil, newConfigError(path, field.key, "unknown rule field %q", key)
			}
		}
		if useField, ok := fields["use"]; ok {
			if len(fields) != 1 {
				return nil, newConfigError(path, useField.key, "use rule must not contain other fields")
			}
			use, err := readString(path, useField.value, "rule set reference")
			if err != nil {
				return nil, err
			}
			if use == "" {
				return nil, newConfigError(path, useField.value, "rule set reference must not be empty")
			}
			rules = append(rules, RawRule{Use: use})
			continue
		}

		idField, ok := fields["id"]
		if !ok {
			return nil, newConfigError(path, item, "rule is missing required field %q", "id")
		}
		id, err := readString(path, idField.value, "rule id")
		if err != nil {
			return nil, err
		}
		if id == "" {
			return nil, newConfigError(path, idField.value, "rule id must not be empty")
		}
		allowField, hasAllow := fields["allow"]
		denyField, hasDeny := fields["deny"]
		if hasAllow == hasDeny {
			return nil, newConfigError(path, item, "rule %q must specify exactly one of allow or deny", id)
		}
		rule := RawRule{ID: id}
		if hasAllow {
			rule.Allow, err = readStringSequence(path, allowField.value, "allow selectors")
			if err != nil {
				return nil, err
			}
			if len(rule.Allow) == 0 {
				return nil, newConfigError(path, allowField.value, "rule %q allow must not be empty", id)
			}
		}
		if hasDeny {
			rule.Deny, err = readStringSequence(path, denyField.value, "deny selectors")
			if err != nil {
				return nil, err
			}
			if len(rule.Deny) == 0 {
				return nil, newConfigError(path, denyField.value, "rule %q deny must not be empty", id)
			}
		}
		if field, ok := fields["message"]; ok {
			rule.Message, err = readString(path, field.value, "rule message")
			if err != nil {
				return nil, err
			}
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func readStringSequence(path string, node *yaml.Node, name string) ([]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, newConfigError(path, node, "%s must be a sequence", name)
	}
	out := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		value, err := readString(path, item, name)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func readString(path string, node *yaml.Node, name string) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", newConfigError(path, node, "%s must be a string", name)
	}
	return node.Value, nil
}

func readInt(path string, node *yaml.Node, name string) (int, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
		return 0, newConfigError(path, node, "%s must be an integer", name)
	}
	var value int
	if err := node.Decode(&value); err != nil {
		return 0, newConfigError(path, node, "%s must be an integer", name)
	}
	return value, nil
}

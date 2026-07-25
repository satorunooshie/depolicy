# Depolicy

Depolicy is a Go static analysis tool for enforcing package dependency policies inside a single Go module.

It reads a declarative `.depolicy.yaml` file, classifies each import as `std:`, `local:`, or `external:`, and reports imports that violate the selected policy for the importing package.

For example, in a module named `github.com/example/backend`, this import:

```go
package handler

import "github.com/example/backend/infra/database"
```

is evaluated as:

```text
local:api/main/handler -> local:infra/database
```

Depolicy can run as a normal CLI command or as a `go vet` vet tool.

## Features

Depolicy supports:

- strict `.depolicy.yaml` parsing
- duplicate YAML key detection
- unknown field detection
- `package-sets`
- `rule-sets`
- selector matching with `...`, `*`, and `{variable}`
- policy assignment checks
- import denial diagnostics
- uncovered package diagnostics
- ambiguous policy diagnostics
- text and JSON output
- `go vet -vettool` usage

## Installation

From this repository:

```bash
go install ./cmd/depolicy
```

From a published module version:

```bash
go install github.com/satorunooshie/depolicy/cmd/depolicy@latest
```

The command installs one binary:

```text
depolicy
```

## Quick Start

Place `.depolicy.yaml` next to the module's `go.mod`.

```text
repository/
|-- go.mod
|-- .depolicy.yaml
|-- api/
|-- domain/
|-- infra/
`-- usecase/
```

Validate the configuration:

```bash
depolicy validate
```

Check all packages:

```bash
depolicy check ./...
```

Use Depolicy as a vet tool:

```bash
go vet -vettool="$(which depolicy)" ./...
```

Explain one concrete dependency:

```bash
depolicy explain \
  --package local:api/main/handler \
  --import local:infra/database
```

## Complete Example

This configuration is a practical starting point for a layered Go service.

```yaml
version: 1

package-sets:
  generated:
    - local:generated/...

  domain-shared:
    - local:domain/shared/...

rule-sets:
  api-common:
    - id: deny-generated
      deny:
        - set:generated

policies:
  - id: api-handler
    packages:
      - local:api/*/handler/...
    imports:
      default: allow
      rules:
        - use: api-common
        - id: deny-infra
          deny:
            - local:infra/...
        - id: deny-usecase
          deny:
            - local:usecase/...

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

  - id: core
    packages:
      - local:core/...
    imports:
      default: deny
      rules:
        - id: allow-standard-library
          allow:
            - std:...

  - id: infra
    packages:
      - local:infra/...
    imports:
      default: allow
```

This example shows three common policy styles:

- API handlers use `default: allow` and deny specific architectural boundaries.
- Domain packages use a path variable to allow same-context imports and deny cross-context imports.
- Core packages use `default: deny` and explicitly allow only the standard library.

## Configuration

The default configuration file is:

```text
.depolicy.yaml
```

Depolicy searches for this file from the current working directory upward. You can also pass it explicitly:

```bash
depolicy check --config ./.depolicy.yaml ./...
```

A minimal configuration looks like this:

```yaml
version: 1

package-sets: {}

rule-sets: {}

policies:
  - id: core
    packages:
      - local:core/...
    imports:
      default: deny
      rules:
        - id: allow-standard-library
          allow:
            - std:...
```

This policy means that packages under `local:core/...` may import only standard library packages.

### Configuration Schema

The top-level configuration fields are:

| Field | Required | Meaning |
| --- | --- | --- |
| `version` | Yes | Configuration schema version. The supported value is `1`. |
| `package-sets` | No | Named groups of package selectors used by import rules. |
| `rule-sets` | No | Named groups of ordered import rules. |
| `policies` | Yes | Dependency policies applied to local source packages. |

Unknown fields are configuration errors.

Depolicy intentionally accepts a strict YAML subset:

- mappings
- sequences
- strings
- booleans
- integers
- null

The following are rejected:

- YAML syntax errors
- duplicate keys
- unknown fields
- missing required fields
- invalid value types
- anchors
- aliases
- merge keys
- custom tags
- multiple YAML documents

Configuration files are not allowed to read external files or expand environment variables.

## Package Classification

Depolicy classifies import targets into three package kinds:

| Kind | Example | Meaning |
| --- | --- | --- |
| `std:` | `std:fmt` | Go standard library package |
| `local:` | `local:domain/order` | Package inside the current module |
| `external:` | `external:github.com/google/uuid` | Package outside the current module |

Named package sets are referenced with `set:`:

```yaml
deny:
  - set:generated
```

## Selectors

Selectors describe package paths.

Exact match:

```yaml
local:domain/order
```

Package and all descendants:

```yaml
local:domain/order/...
```

One path segment:

```yaml
local:api/*/handler/...
```

Path variable:

```yaml
local:domain/{context}/...
```

Path variables are captured from `policies[].packages` and can be reused in rules inside the same policy.

```yaml
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
        - id: deny-other-contexts
          deny:
            - local:domain/...
```

If the importing package is `local:domain/order/service`, then `{context}` is `order`. Imports from `local:domain/order/...` are allowed by the first rule. Imports from another domain context, such as `local:domain/user/entity`, are denied by the second rule.

## Package Sets

Use `package-sets` to name reusable groups of import targets.

```yaml
package-sets:
  generated:
    - local:generated/sqlc/...
    - local:generated/mock/...

  domain-shared:
    - local:domain/shared/...
```

Package sets may reference other package sets:

```yaml
package-sets:
  generated:
    - local:generated/sqlc/...

  restricted:
    - set:generated
    - local:internal/database/...
```

Package sets are used by import rules, not by `policies[].packages`.

## Rule Sets

Use `rule-sets` to reuse ordered import rules.

```yaml
rule-sets:
  api-common:
    - id: deny-generated
      deny:
        - set:generated

    - id: deny-components
      deny:
        - local:component/...
```

Reference a rule set with `use`:

```yaml
policies:
  - id: api-handler
    packages:
      - local:api/*/handler/...
    imports:
      default: allow
      rules:
        - use: api-common
        - id: deny-infra
          deny:
            - local:infra/...
```

Rules are evaluated from top to bottom after rule-set expansion. The first matching rule decides whether the import is allowed or denied. If no rule matches, `imports.default` is used.

## Adoption Guide

For an existing codebase, start with a permissive configuration and tighten it gradually.

1. Add `.depolicy.yaml` next to `go.mod`.
2. Add one policy for each major source tree, such as `api`, `domain`, `infra`, and `usecase`.
3. Start most policies with `default: allow`.
4. Run `depolicy check ./...`.
5. Fix every `uncovered-package` diagnostic by adding an explicit policy.
6. Add focused `deny` rules for dependencies that should never cross a boundary.
7. Move repeated rules into `rule-sets`.
8. Move repeated target selectors into `package-sets`.
9. Use `default: deny` only for small, stable areas where every allowed dependency is intentional.

This keeps the first rollout small. You can enforce broad architectural rules after the project is fully covered by policies.

## Common Recipes

### Deny handler-to-infra imports

```yaml
policies:
  - id: api-handler
    packages:
      - local:api/*/handler/...
    imports:
      default: allow
      rules:
        - id: deny-infra
          deny:
            - local:infra/...
```

### Deny generated code imports outside infrastructure

```yaml
package-sets:
  generated:
    - local:generated/...

policies:
  - id: usecase
    packages:
      - local:usecase/...
    imports:
      default: allow
      rules:
        - id: deny-generated
          deny:
            - set:generated
```

### Deny cross-domain imports

```yaml
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
        - id: deny-other-contexts
          deny:
            - local:domain/...
```

### Allow only standard library imports

```yaml
policies:
  - id: core
    packages:
      - local:core/...
    imports:
      default: deny
      rules:
        - id: allow-standard-library
          allow:
            - std:...
```

### Deny one external package family

```yaml
policies:
  - id: integration
    packages:
      - local:integration/...
    imports:
      default: allow
      rules:
        - id: deny-legacy-sdk
          deny:
            - external:github.com/example/legacy-sdk/...
```

## Diagnostics

Depolicy reports at least these diagnostic codes:

| Code | Meaning |
| --- | --- |
| `import-denied` | An import is denied by a policy |
| `uncovered-package` | A local package does not match any policy |
| `ambiguous-policy` | A local package matches multiple policies |
| `invalid-config` | The configuration is invalid |
| `package-load-error` | Go package loading failed |

Text output is intended for humans and CI logs:

```text
api/main/handler/user.go:12:2: import "github.com/example/backend/infra/database" is denied by policy "api-handler" rule "deny-infra"

  package: local:api/main/handler
  import:  local:infra/database
  policy:  api-handler
  rule:    deny-infra
  matched: local:infra/...
```

JSON output is available with:

```bash
depolicy check --format=json ./...
```

## Troubleshooting

### `uncovered-package`

The package being checked does not match any `policies[].packages` selector.

Fix it by adding a policy for that package tree:

```yaml
policies:
  - id: usecase
    packages:
      - local:usecase/...
    imports:
      default: allow
```

### `ambiguous-policy`

The package matches more than one policy. Depolicy does not merge policies and does not use declaration order as priority.

For example, this is ambiguous for `local:api/main/handler`:

```yaml
policies:
  - id: api
    packages:
      - local:api/...
    imports:
      default: allow

  - id: api-handler
    packages:
      - local:api/*/handler/...
    imports:
      default: deny
```

Fix it by changing the package selectors so each concrete package matches exactly one policy.

### `invalid-config`

The configuration failed strict parsing or schema validation. Common causes are:

- misspelled fields
- duplicate YAML keys
- empty rule lists
- a rule containing both `allow` and `deny`
- `set:` references to missing package sets
- `use` references to missing rule sets
- path variables used outside the policy that defines them

Run:

```bash
depolicy validate --config ./.depolicy.yaml
```

### Unexpected default denial

If an import is denied without a rule ID, it was denied by `imports.default`.

Use `explain` to inspect the decision:

```bash
depolicy explain \
  --package local:core/logging \
  --import local:domain/user
```

## Exit Codes

When run as a CLI command, Depolicy uses these exit codes:

| Code | Meaning |
| ---: | --- |
| `0` | Success |
| `1` | Policy violation or policy assignment error |
| `2` | Configuration error |
| `3` | Runtime error |

When run as a vet tool, Depolicy follows the `go vet` execution model.

## Example Project

The example fixture under [testdata/project](./testdata/project) is intentionally small but covers the main policy features:

- rule sets
- package sets
- path variables
- default allow policies
- default deny policies
- standard library selectors
- local package selectors
- external package selectors
- uncovered packages
- ambiguous policy assignment

Run it directly:

```bash
depolicy check --config testdata/project/.depolicy.yaml ./...
```

Or as a vet tool:

```bash
cd testdata/project
go vet -vettool="$(which depolicy)" ./...
```

The fixture intentionally contains violations so that expected diagnostics are visible.

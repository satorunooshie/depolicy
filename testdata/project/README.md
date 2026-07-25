# Depolicy Example Project

This module is a compact example project for Depolicy. It is used by the test suite, and it is also meant to be read as a practical reference for writing `.depolicy.yaml`.

The project intentionally contains policy violations. Running Depolicy against this module should produce diagnostics for denied imports, uncovered packages, and ambiguous policy assignment.

## Run the Example

From the repository root:

```bash
depolicy check --config testdata/project/.depolicy.yaml ./...
```

As a vet tool:

```bash
cd testdata/project
go vet -vettool="$(which depolicy)" ./...
```

## What this Project Demonstrates

### API Handler Policy

`api/main/handler` is covered by the `api-handler` policy.

It demonstrates:

- `rule-sets` through `use: api-common`
- `package-sets` through `set:generated`
- `std:` denial with `std:errors`
- local layer denial with `local:infra/...`
- component denial with `local:component/...`

### Component Policy

`component/{component}` is covered by the `component` policy.

It demonstrates path variables. A package under `local:component/billing/...` may import another package under `local:component/billing/...`, but it may not import `local:component/user/...`.

### Domain Context Policy

`domain/{context}` is covered by the `domain-context` policy.

It demonstrates same-context domain imports. A package under `local:domain/order/...` may import `local:domain/order/...`, but cross-context imports such as `local:domain/user/...` are denied.

The `domain-shared` package set is allowed as an exception for shared domain values.

### Core Policy

`core` uses `default: deny`.

It allows only the standard library through:

```yaml
allow:
  - std:...
```

Any local or external import not explicitly allowed is denied by the default decision.

### Usecase and Infra Policies

`usecase` and `infra` use `default: allow` with targeted denial rules.

They demonstrate the common "allow most imports, deny architectural boundaries" style.

### Generated Packages

`generated` is covered by a permissive policy so generated packages themselves are not reported as uncovered.

Other layers still deny imports to generated code through:

```yaml
set:generated
```

### External Package Policy

`integration` demonstrates `external:` selectors.

The module `example.com/ext` is provided locally through a `replace` directive in `go.mod`. The policy allows `example.com/ext/allowed` and denies `example.com/ext/forbidden`.

### Uncovered Package

`uncovered/service` intentionally has no matching policy.

It demonstrates the `uncovered-package` diagnostic.

### Ambiguous Policy Assignment

`ambiguous/service` intentionally matches two policies:

- `ambiguous-wide`
- `ambiguous-service`

It demonstrates the `ambiguous-policy` diagnostic. Depolicy does not merge policies and does not use declaration order as priority. A concrete package must match exactly one policy.

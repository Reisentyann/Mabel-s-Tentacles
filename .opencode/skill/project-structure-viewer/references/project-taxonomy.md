# Project Taxonomy

Use this reference when a project is unfamiliar, mixed, or not a conventional web app. Identify the project from evidence in manifests, entry points, configs, tests, and docs. Do not force every repository into frontend/backend phases.

## Universal Surfaces

- **Orientation**: `README*`, `docs/`, `examples/`, `CHANGELOG`, `CONTRIBUTING`, `AGENTS.md`, `CLAUDE.md`, `CODEX.md`.
- **Manifests and dependencies**: language package manifests, lockfiles, workspace files, build files.
- **Runtime entries**: files that start services, CLIs, jobs, apps, notebooks, model pipelines, or library exports.
- **Core logic**: `src/`, `lib/`, `core/`, `domain/`, `internal/`, `pkg/`, `crates/`, `packages/`.
- **Interfaces and adapters**: APIs, routers, controllers, handlers, commands, events, SDK surfaces, UI screens.
- **Data and contracts**: schemas, migrations, SQL, protobuf, GraphQL, OpenAPI, model configs, fixtures.
- **Validation**: `test/`, `tests/`, `spec/`, `fixtures/`, CI workflows, benchmark scripts.
- **Operations**: Docker, Compose, Terraform, Helm, Kubernetes, deployment scripts, Makefiles.

## Common Project Types

### JavaScript / TypeScript

Read `package.json`, workspace files, `tsconfig*`, bundler configs, entry files under `src/`, and test configs. For monorepos, identify apps/packages first, then sample representative package manifests.

### Python

Read `pyproject.toml`, `requirements*.txt`, `setup.py`, `tox.ini`, `noxfile.py`, `conftest.py`, and entry files such as `__main__.py`, `cli.py`, `app.py`, `server.py`, `manage.py`, `train.py`, `inference.py`, or package `__init__.py` exports.

### Go

Read `go.mod`, `cmd/*`, `main.go`, `internal/`, `pkg/`, API handlers, generated protobuf boundaries, and tests near core packages.

### Rust

Read `Cargo.toml`, workspace members, `src/main.rs`, `src/lib.rs`, `crates/*`, feature flags, examples, benches, and integration tests.

### Java / Kotlin / JVM

Read `pom.xml` or Gradle files, module layout, `src/main`, `src/test`, application bootstrap classes, controller/service/repository layers, and generated resources.

### Swift / iOS / macOS

Read `Package.swift` or Xcode project metadata, app entry points, SwiftUI/AppKit/UIKit roots, model/services directories, resources, tests, and build/signing configs.

### Mobile / Flutter / React Native

Read `pubspec.yaml` or mobile package manifests, platform directories, app entry point, navigation/root widgets, native bridges, assets, and test directories.

### Data / ML / Analytics

Read notebooks, `train.py`, `predict.py`, `inference.py`, pipelines, dataset configs, model configs, feature code, evaluation scripts, experiment tracking configs, and data contracts.

### Infrastructure / DevOps

Read Terraform roots/modules, Helm charts, Kubernetes manifests, Dockerfiles, Compose files, CI/CD workflows, deployment scripts, and environment templates.

### Documentation / Content

Read site config, content directories, theme/layout files, examples, generated asset rules, and build/deploy scripts.

## Phase Map Pattern

When summarizing, prefer phases such as:

1. Orientation and intent
2. Build/dependency setup
3. Runtime or authoring entry points
4. Core domain/system logic
5. Interfaces, adapters, and user surfaces
6. Data, schemas, contracts, and assets
7. Tests, quality gates, and examples
8. Deployment, infrastructure, and automation

Rename or merge phases to match the project. A CLI library, Terraform module, ML notebook repo, or docs site should not be described as frontend/backend unless the files support that.

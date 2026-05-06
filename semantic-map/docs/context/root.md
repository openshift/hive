# Repository manifest

## High-level Purpose

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/openshift/hive)

## Tech Stack

- **Language:** Go
- **Module:** `github.com/openshift/hive`
- **Shell scripts:** 25 file(s) — summary in **`go-facts.json`** (`polyglot.bash`).
- **Python:** 4 file(s) — summary in **`go-facts.json`** (`polyglot.python`).
- **Also at repo root:** (no ext) (5 file(s) at repo root)
- **Also at repo root:** .json (1 file(s) at repo root)
- **Also at repo root:** .ote (1 file(s) at repo root)
- **Also at repo root:** .yml (1 file(s) at repo root)

## Directory Map

- **.ai/** — see package map and imports for detail.
- **.tekton/** — see package map and imports for detail.
- **apis/** — Go sources present (4 `.go` file(s) directly under this folder).
- **build/** — see package map and imports for detail.
- **cmd/** — `main` packages / CLI binaries (hiveadmission, hiveutil, manager, operator, waitforjob).
- **config/** — configuration manifests or samples.
- **contrib/** — see package map and imports for detail.
- **docs/** — documentation.
- **hack/** — supporting scripts for development or CI.
- **overlays/** — see package map and imports for detail.
- **pkg/** — library packages imported by other parts of the repo.
- **semantic-map/** — see package map and imports for detail.
- **test/** — integration or auxiliary tests.
- **.codecov.yml** — top-level file.
- **.coderabbit.yaml** — top-level file.
- **.gitignore** — top-level file.
- **.snyk** — top-level file.
- **AGENTS.md** — top-level file.
- **CLAUDE.md** — top-level file.
- **CONTRIBUTING.md** — top-level file.
- **Dockerfile** — top-level file.
- **Dockerfile.ote** — top-level file.
- **LICENSE** — top-level file.
- **Makefile** — build entry.
- **OWNERS** — top-level file.
- **PROJECT** — top-level file.
- **README.md** — top-level file.
- **go.mod** — Go module definition.
- **go.sum** — Go module checksums.
- **golangci.yml** — top-level file.
- **renovate.json** — top-level file.

## Entry Points

- `github.com/openshift/hive/cmd/hiveadmission`
- `github.com/openshift/hive/cmd/manager`
- `github.com/openshift/hive/cmd/operator`
- `github.com/openshift/hive/contrib/cmd/hiveutil`
- `github.com/openshift/hive/contrib/cmd/waitforjob`
- `github.com/openshift/hive/hack`

## Understanding Score

1.0

_Generated deterministically from repository layout and go/packages; narrative intent not audited._

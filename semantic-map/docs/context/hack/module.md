# Module atlas

## Responsibility

Go-based CI utility (`modcheck.go`) that verifies the `apis/go.mod` file stays in sync with the root `go.mod` — comparing require, exclude, and replace directives. Also houses shell scripts, Python helpers, and configuration for build, test, and operational tasks.

## Public Interface/API

- `main()` — CLI entry point; accepts optional `-f` flag to auto-fix `apis/go.mod` require versions
- No exported library API (package main)

## Internal Dependencies

- `golang.org/x/mod/modfile` — go.mod file parsing and manipulation (external)

## Capabilities

- Compares root `go.mod` and `apis/go.mod` require/exclude/replace directives
- Auto-fixes out-of-sync require versions in `apis/go.mod` when run with `-f`
- Reports mismatches with clear XX-prefixed output per directive type
- Contains shell scripts for e2e testing, kind cluster setup, scale testing, cert generation, and various operational tasks (not analyzed as Go)

## Understanding Score

0.9

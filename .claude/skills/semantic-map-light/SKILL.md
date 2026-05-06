---
name: semantic-map-light
description: >-
  Application repo: read semantic-map/docs/context and JSON while coding (bugs, features).
  Map authoring (analyze, orchestrate, augment module.md) runs from the semantic-map tool
  checkout—see semantic-map/ANALYSIS.md and upstream docs.
---

# Semantic map (light — application bundle)

This skill covers **using** a checked-in bundle **while you change code** (read **`docs/context/`**, **`go-facts.json`**, etc.). It does **not** document the full **map build** pipeline—that lives in the **semantic-map** tool repository (**`analyze`**, **`orchestrate`**, Claude augmentation recipes). Your team’s **`semantic-map/ANALYSIS.md`** points at that workflow; refreshing facts still requires the **`semantic-map`** binary from **that** checkout.

## Bundle location

After adopting the **semantic-map** tool (this application repo’s `semantic-map/ANALYSIS.md` records which fork/version you use), the artifact tree is typically:

```text
semantic-map/                        # bundle root (<clone>/semantic-map after analyze)
```

- **`docs/context/root.md`** — L0 atlas (start here).
- **`docs/context/<path>/module.md`** — L1 per Go source directory.
- **`go-facts.json`**, **`repo-tree.json`**, **`deps-graph.json`** — deterministic inputs for agents.
- **`prompts/`** — pinned copies of Architect / Analyst / Auditor templates used when this bundle was generated.
- **`manifest.json`** — bundle metadata; v2 uses **bundle-relative** `clone_dir` / `artifact_root` (resolved at `prompts expand` time from the bundle path you pass in).

The **semantic-map tool repository** may also ship **example** trees under **`map/<owner>/<repo>/map/`**; that layout is not what **`analyze`** emits by default.

## Read-first workflow (no tool install)

1. Open **`docs/context/root.md`** for system overview.
2. Jump to the relevant **`module.md`** under **`docs/context/`** matching the package path.
3. Cross-check exports and imports with **`go-facts.json`** / **`deps-graph.json`** when precision matters.

## When you need the semantic-map CLI

You need a built **`semantic-map` binary** and the upstream tool checkout (or install path) to:

- Run **`analyze`** again after large code moves, new `cmd/` trees, or to refresh JSON facts.
- Run **`orchestrate`**, **`index`**, **`validate`**, or **`query`** on the bundle.

The canonical flags for this repo should live in **`semantic-map/ANALYSIS.md`** (or the path your team chose when copying **`contrib/target-repo/`** from the semantic-map tool repository).

## Prompt expansion

**`prompts expand`** reads templates from **`prompts/`** inside the bundle, not from the tool repo. You still need the **binary** and a **`manifest.json`** whose paths match the current machine (see ANALYSIS.md portable manifests note).

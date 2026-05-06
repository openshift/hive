# Canonical semantic-map analysis recipe

Run from a machine that has the **semantic-map** repository built (`make build` → `bin/semantic-map`). Run **`analyze`** from the **semantic-map** checkout so **`prompts/`** can be copied (or pass **`-prompts-from`**).

| Placeholder | Meaning |
|-------------|---------|
| `CLONE` | Absolute path to your **local clone** of this application repository |
| `OWNER` | GitHub/GitLab org or user (first segment of `owner/repo`) |
| `REPO` | Repository name (second segment of `owner/repo`) |
| `SEMANTIC_MAP` | Absolute path to your **semantic-map** tool checkout (contains `Makefile`, `prompts/`) |

**Default:** artifacts go under **`CLONE/semantic-map/`** (bundle root).

## One-shot refresh (facts, stubs, index, orchestration)

```bash
cd "$SEMANTIC_MAP"
make build

./bin/semantic-map analyze "$CLONE" \
  --slug OWNER/REPO \
  --write-facts \
  --refresh-root \
  --refresh-modules \
  -markdown-index

BUNDLE="$CLONE/semantic-map"

./bin/semantic-map orchestrate "$BUNDLE"
./bin/semantic-map index "$BUNDLE"
```

Derive `OWNER/REPO` from `git -C "$CLONE" remote get-url origin` if you omit **`--slug`**.

**Custom bundle location:**

```bash
./bin/semantic-map analyze "$CLONE" -output "$HOME/semantic-maps/OWNER/REPO" --write-facts
# set BUNDLE to that -output path for orchestrate/index
```

## Optional: expand prompts only (binary + bundle)

Uses **`bundle/prompts/`** from the analyzed tree.

```bash
./bin/semantic-map prompts expand "$BUNDLE"
```

## Portable manifests

New **`analyze`** output uses **`manifest.json` version 2** with **bundle-relative** `clone_dir` / `artifact_root` when the bundle lives under the clone (default **`<clone>/semantic-map/`**). **`prompts expand <bundle>`** resolves paths using the bundle directory you pass in.

Older bundles may still be **version 1** with absolute paths; re-run **`analyze`** to refresh.


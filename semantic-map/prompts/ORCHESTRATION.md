# Orchestration

Use this sequence when driving agents by hand. The CLI can emit a **checklist with paths and copy-paste commands**:

```bash
./bin/semantic-map orchestrate map/<owner>/<repo>/map
```

That writes **`orchestration-queue.md`** next to the bundle (and runs **`prompts expand`** first unless you pass **`-expand=false`**).

1. **Analyze (tool)** — From the `semantic-map` repo, point at a **local clone**:
   ```bash
   make build
   ./bin/semantic-map analyze /path/to/clone --write-facts
   ```
   This produces under `map/<owner>/<repo>/map/`:
   - `docs/context/` — `root.md`, `module.md` stubs
   - `go-facts.json`, `repo-tree.json`, **`deps-graph.json`**, **`manifest.json`**
   - `prompts/` — copies of templates from this directory (if `--bundle-prompts`)

   Optional: substitute prompt placeholders:
   ```bash
   ./bin/semantic-map prompts expand map/<owner>/<repo>/map
   ```
   Use files under **`prompts-expanded/`** when pasting into an LLM.

2. **Architect (L0)** — Feed **`repo-tree.json`**, **`go-facts.json`** (summary), **`deps-graph.json`** (import classes), and **`architect.md`** (expanded); refine **`docs/context/root.md`** where the stub is incomplete.

3. **Analyst (L1)** — For each important directory, use **`analyst.md`** with that path and deterministic facts; refine **`module.md`** files.

4. **Specialist (L2)** — For hot files, use **`specialist.md`** to author **`docs/context/.../<file>.md`** per [concept.md](../concept.md).

5. **Auditor** — Run **`auditor.md`** against each `module.md` / file map you care about; adjust understanding scores and fix discrepancies.

6. **Validate** — `semantic-map validate map/<owner>/<repo>/map/docs/context`

Iterate between L1/L2 depth and narrative length as needed for your repo size.

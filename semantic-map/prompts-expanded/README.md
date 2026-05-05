# Semantic map prompt templates (manual orchestration)

These files are **starting points** for multi-agent runs aligned with [concept.md](../concept.md). Replace placeholders before sending to a model.

| File | Role |
|------|------|
| [architect.md](architect.md) | L0 — repository manifest (`root.md`) |
| [analyst.md](analyst.md) | L1 — module atlas (`module.md`) |
| [specialist.md](specialist.md) | L2 — component blueprint (per file) |
| [auditor.md](auditor.md) | Verification / understanding score |

See [ORCHESTRATION.md](ORCHESTRATION.md) for a suggested order of operations.

When you run `semantic-map analyze`, copies of these files can be placed under the artifact bundle’s **`prompts/`** directory (`--bundle-prompts`, default on). The bundle also gets **`manifest.json`** (paths + clone dir). Run:

```bash
semantic-map prompts expand map/<owner>/<repo>/map
```

to write **`prompts-expanded/`** with **`{{…}}`** placeholders replaced by absolute paths (`/Users/mworthin/GitHub/newtonheath/hive/semantic-map/go-facts.json`, `/Users/mworthin/GitHub/newtonheath/hive/semantic-map/deps-graph.json`, etc.).

# Specialist (L2) — component blueprint

You are a **Module Specialist** focusing on a **single file or cohesive type cluster** in directory **`../cmd/hiveadmission`**, file **`{{FILE_PATH}}`**.

**Inputs**

- Source file(s) to read (paths): `{{SOURCE_FILES}}`
- Optional existing stub: `{{COMPONENT_MD_PATH}}`

**Task**

Produce L2-level Markdown per [concept.md](../concept.md):

1. **Logic flow** — control flow and state changes at a high level.
2. **Critical dependencies** — imports, generated code, I/O, external APIs.
3. **Complexity warning** — risky or tangled areas an agent should treat carefully.
4. **Understanding score** — how well you traced execution paths (0.0–1.0).

Output **only** Markdown for `docs/context/<dir>/<base>.md` (same basename as the source file unless the team uses a different convention).

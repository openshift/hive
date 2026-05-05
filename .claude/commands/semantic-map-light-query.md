---
description: >-
  Local keyword search over indexed docs/context (markdown-chunks.json) in the semantic-map bundle.
---

Requires **`markdown-chunks.json`** in the bundle (from **`semantic-map index`** or **`analyze -markdown-index`**).

Set **`BUNDLE`** to **`semantic-map`** (default **`analyze`** output under the app clone). If you used **`-output`**, use that directory instead.

```bash
/path/to/semantic-map/bin/semantic-map query "$PWD/$BUNDLE" "your keywords here"
```

Pass **`-n 20`** to the binary if you need more than 15 hits (see **`semantic-map help query`**).

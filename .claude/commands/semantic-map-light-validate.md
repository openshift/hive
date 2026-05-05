---
description: >-
  Validate semantic map Markdown under the bundle docs/context (relative bundle root, usually semantic-map/).
---

Set **`BUNDLE`** to **`semantic-map`** (default **`analyze`** layout) or the path you passed to **`-output`**.

From a machine with **semantic-map** built:

```bash
make -C /path/to/semantic-map build
/path/to/semantic-map/bin/semantic-map validate "$PWD/$BUNDLE/docs/context"
```

Replace **`BUNDLE`** with the relative path under this repo’s root. Exit code **1** means missing required `##` sections per **`concept.md`** in your semantic-map tool checkout.

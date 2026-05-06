<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/operator/assets/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Asset` — Asset loads and returns the asset for the given name. It returns an error if the asset could not be found or could not be loaded.
- `AssetDir` — AssetDir returns the file names below a certain directory embedded in the file by go-bindata. For example if you run go-bindata on data/... and data contains the following hierarc…
- `AssetInfo` — AssetInfo loads and returns the asset info for the given name. It returns an error if the asset could not be found or could not be loaded.
- `AssetNames` — AssetNames returns the names of the assets.
- `MustAsset` — MustAsset is like Asset but panics when Asset would return an error. It simplifies safe initialization of global variables.
- `RestoreAsset` — RestoreAsset restores an asset under the given directory
- `RestoreAssets` — RestoreAssets restores an asset under the given directory recursively

## Internal Dependencies

- `fmt`
- `io/ioutil`
- `os`
- `path/filepath`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **assets**.
- Go **`import`** edges listed below (6 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/operator/assets`.

## Understanding Score

0.0

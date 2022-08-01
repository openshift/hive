# yaml-patch

### **Note: This repo is for all intents and purposes abandoned. I would suggest using [ytt](https://get-ytt.io/) instead!**

`yaml-patch` is a version of Evan Phoenix's
[json-patch](https://github.com/evanphx/json-patch), which is an implementation
of [JavaScript Object Notation (JSON) Patch](https://tools.ietf.org/html/rfc6902),
directly transposed to YAML.


## Syntax

General syntax is the following:

```yaml
- op: <add | remove | replace | move | copy | test>
  from: <source-path> # only valid for the 'move' and 'copy' operations
  path: <target-path> # always mandatory
  value: <any-yaml-structure> # only valid for 'add', 'replace' and 'test' operations
```

### Paths

Supported YAML path are primarily those of
[RFC 6901 JSON Pointers](https://tools.ietf.org/html/rfc6901).

A syntax extention with `=` was added to match any sub-element in a YAML
structure by key/value.

For example, the following removes all sub-nodes of the `releases` array that
have a `name` key with a value of `cassandra`:

```yaml
- op: remove
  path: /releases/name=cassandra
```

A major caveat with `=`, is that it actually performs a _recursive_ search for
matching nodes. The root node at which the recursive search is initiated, is
the node matched by the path prefix before `=`.

The second caveat is that the recursion stops at a matching node. With the
`add` operation, you could expect sub-nodes of matching nodes to also match,
but they don't.

If your document is the following and you apply the patch above, then all
sub-nodes of `/releases` that match `name=cassandra` will be removed.

```yaml
releases: # a recursive search is made, starting from this node
  - name: cassandra # does match, will be removed
  - - name: toto
    - name: cassandra # does match, will be removed!
      sub:
        - name: cassandra # not matched: the recursion stops at matching parent node
  - super:
      sub:
        name: cassandra # does match, will be removed!
```

#### Path Escaping

As in RFC 6901, escape sequences are introduced by `~`. So, `~` is escaped
`~0`, `/` is escaped `~1`. There is no escape for `=` yet.


### Operations

Supported patch operations are those of [RFC 6902](https://tools.ietf.org/html/rfc6902).

- [`add`](https://tools.ietf.org/html/rfc6902#section-4.1)
- [`remove`](https://tools.ietf.org/html/rfc6902#section-4.2)
- [`replace`](https://tools.ietf.org/html/rfc6902#section-4.3)
- [`move`](https://tools.ietf.org/html/rfc6902#section-4.4)
- [`copy`](https://tools.ietf.org/html/rfc6902#section-4.5)
- [`test`](https://tools.ietf.org/html/rfc6902#section-4.6)


## Installing

`go get github.com/krishicks/yaml-patch`

If you want to use the CLI:

`go get github.com/krishicks/yaml-patch/cmd/yaml-patch`

## API

Given the following RFC6902-ish YAML document, `ops`:

```
---
- op: add
  path: /baz/waldo
  value: fred
```

And the following YAML that is to be modified, `src`:

```
---
foo: bar
baz:
  quux: grault
```

Decode the ops file into a patch:

```
patch, err := yamlpatch.DecodePatch(ops)
// handle err
```

Then apply that patch to the document:

```
dst, err := patch.Apply(src)
// handle err

// do something with dst
```

### Example

```
doc := []byte(`---
foo: bar
baz:
  quux: grault
`)

ops := []byte(`---
- op: add
  path: /baz/waldo
  value: fred
`)

patch, err := yamlpatch.DecodePatch(ops)
if err != nil {
  log.Fatalf("decoding patch failed: %s", err)
}

bs, err := patch.Apply(doc)
if err != nil {
  log.Fatalf("applying patch failed: %s", err)
}

fmt.Println(string(bs))
```

```
baz:
  quux: grault
  waldo: fred
foo: bar
```

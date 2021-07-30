# Deep hash

[![Build Status](https://travis-ci.org/davegardnerisme/deephash.svg?branch=master)](https://travis-ci.org/davegardnerisme/deephash)

A library for calculating a deterministic hash for simple or nested data structures
in Go. The traversal algorithm is based on [mergo](https://github.com/imdario/mergo),
which is in turn based on `reflect/deepequal.go` from the stdlib.

Example:

```
eg1 := "foo"
eg2 := example{
	Foo: "foo",
	Bar: 43.0,
}
eg3 := &example{
	Foo: "foo",
	Bar: 43.0,
}

fmt.Printf("String\t%x\n", deephash.Hash(eg1))
fmt.Printf("Struct\t%x\n", deephash.Hash(eg2))
fmt.Printf("Pointer\t%x\n", deephash.Hash(eg3))
```

Output:

```
String	dcb27518fed9d577
Struct	e0979b89bf545866
Pointer	e0979b89bf545866
```

It's worth noting that here two structs with the same content (eg: a copy),
where one is a pointer and one isn't, **both hash to the same value**.

You can run this via:

```
go run example/example.go
```

## Key features

 - uses fnv 64a hashing algorithm which is fast with good distribution
 - deterministic hashing for maps (which don't have any order) by sorting on
   the hash of the keys

## Docs

http://godoc.org/github.com/davegardnerisme/deephash


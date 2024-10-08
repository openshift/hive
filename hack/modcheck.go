package main

import (
	"fmt"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"os"
)

const (
	rootPath = "go.mod"
	apisPath = "apis/go.mod"
)

func readModfile(path string) *modfile.File {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	f, err := modfile.Parse(path, b, nil)
	if err != nil {
		panic(err)
	}
	return f
}

func writeModfile(path string, modfile *modfile.File, inSync, needsWrite bool) int {
	if needsWrite {
		fmt.Printf("Writing modified %s\n", path)
		modfile.Cleanup()
		b, err := modfile.Format()
		if err != nil {
			fmt.Printf("Couldn't format modified %s: %s\n", path, err)
			return 2
		}
		if err = os.WriteFile(path, b, 0); err != nil {
			fmt.Printf("Failed to write modified %s: %s\n", path, err)
			return 2
		}
		fmt.Printf("\tDone\n")
	}

	if inSync {
		fmt.Printf("%s is in sync\n", path)
		if needsWrite {
			fmt.Printf("\t(after fixing)\n")
		}
		return 0
	} else {
		fmt.Printf("\n%s is out of sync\n", path)
		if needsWrite {
			fmt.Printf("\t(despite partial fixing)\n")
		}
		return 1
	}
}

func main() {
	// TODO: Actual arg parsing
	doFix := false
	if len(os.Args) > 1 && os.Args[1] == "-f" {
		fmt.Printf("\tFixing: enabled\n")
		doFix = true
	}

	rootFile := readModfile(rootPath)
	apisFile := readModfile(apisPath)
	overallInSync := true
	overallNeedsWrite := false

	for _, comparator := range []Comparator{RequireComparator, ExcludeComparator, ReplaceComparator} {
		inSync, needsWrite := comparator.CompareAndFix(rootFile, apisFile, doFix)
		overallInSync = overallInSync && inSync
		overallNeedsWrite = overallNeedsWrite || needsWrite
	}

	os.Exit(writeModfile(apisPath, apisFile, overallInSync, overallNeedsWrite))
}

type Comparator interface {
	// CompareAndFix returns (inSync, needsWrite)
	CompareAndFix(source, target *modfile.File, doFix bool) (bool, bool)
}

// ComparatorImpl is a trait object (you can think of it as an super interface) that also implements Comparator.
// This pattern is necessary because we can't implement local go interfaces on foreign types.
// C is the type we are comparing (and we must supply our own comparison function CompareFn)
// K is a type that is suitable for indexing a map of C (used with FixFn)
type ComparatorImpl[C any, K comparable] struct {
	// helper fns (and optional format string) for constructing a map[K]C
	GetComparablesFn  func(*modfile.File) []C
	KeyComparableFn   func(C) K
	CompareFn         func(C, C) bool
	MapConflictFmtStr string // optional

	// type-specific format strings for logging specific comparison cases
	CompareNotEqualFmtStr string // optional
	// MissingKeyFmtStr        string // optional
	// CompareEquivalentFmtStr string // optional
	// ^ (For a future "verbose mode")

	// fn to apply a given fix
	FixFn func(*modfile.File, K, C) error
}

// CompareAndFix returns (inSync, needsWrite)
// (this is the only implementation of the Comparator trait)
func (t *ComparatorImpl[C, K]) CompareAndFix(checkSource, checkTarget *modfile.File, doFix bool) (bool, bool) {
	sourceMap := t.makeMapFromList(t.GetComparablesFn(checkSource))
	targetMap := t.makeMapFromList(t.GetComparablesFn(checkTarget))
	fixesDelta := t.compare(sourceMap, targetMap)

	fixesRequired := len(fixesDelta)
	inSync := fixesRequired == 0
	needsWrite := false

	if doFix && !inSync {
		fixesApplied := t.fixAll(checkTarget, fixesDelta)
		needsWrite = fixesApplied > 0
		inSync = fixesRequired == fixesApplied
	}

	return inSync, needsWrite
}

// makeMapFromList and all following lowercase private methods are the generic implementation details
func (t *ComparatorImpl[C, K]) makeMapFromList(list []C) map[K]C {
	ret := make(map[K]C, len(list))
	for _, item := range list {
		key := t.KeyComparableFn(item)
		if existingItem, ok := ret[key]; ok && len(t.MapConflictFmtStr) > 0 {
			fmt.Printf(t.MapConflictFmtStr, key, existingItem, item)
		}
		ret[key] = item
	}
	return ret
}

func (t *ComparatorImpl[C, K]) compare(primary, secondary map[K]C) map[K]C {
	delta := make(map[K]C)
	for key, primaryValue := range primary {
		secondaryValue, ok := secondary[key]
		if !ok {
			// For a future "verbose mode":
			// fmt.Printf(t.MissingKeyFmtStr, key)
			continue
		}
		if t.CompareFn(primaryValue, secondaryValue) {
			// For a future "verbose mode":
			// fmt.Printf(t.CompareEquivalentFmtStr, key, primaryValue)
		} else {
			if len(t.CompareNotEqualFmtStr) > 0 {
				fmt.Printf(t.CompareNotEqualFmtStr, key, primaryValue, secondaryValue)
			}
			delta[key] = primaryValue
		}
	}
	return delta
}

func (t *ComparatorImpl[C, K]) fixAll(fixTarget *modfile.File, fixes map[K]C) int {
	fixCount := 0
	for key, value := range fixes {
		if err := t.FixFn(fixTarget, key, value); err == nil {
			fmt.Printf("\tFixed\n")
			fixCount = fixCount + 1
		} else {
			// error already printed
			// keep going (try to fix as many as possible)
		}
	}
	return fixCount
}

// RequireComparator and the Comparators following it are the glue code that we need
// to wire up the foreign types we want to think generically about and the business logic
// we want to be generic over.
var RequireComparator Comparator = &ComparatorImpl[*modfile.Require, string]{
	GetComparablesFn:  func(f *modfile.File) []*modfile.Require { return f.Require },
	KeyComparableFn:   func(r *modfile.Require) string { return r.Mod.Path },
	CompareFn:         func(left, right *modfile.Require) bool { return left.Mod == right.Mod },
	MapConflictFmtStr: "WARNING: require path %s listed at multiple versions: %v | %v\n",

	CompareNotEqualFmtStr: "XX require %s: root(%v) apis(%v)\n",
	FixFn: func(file *modfile.File, toDrop string, toAdd *modfile.Require) error {
		if err := file.DropRequire(toDrop); err != nil {
			fmt.Printf("Error dropping requirement for %s: %s\n", toDrop, err)
			return err
		}
		if err := file.AddRequire(toAdd.Mod.Path, toAdd.Mod.Version); err != nil {
			fmt.Printf("Error adding requirement for %s: %s\n", toDrop, err)
			return err
		}
		return nil
	},
}

var ExcludeComparator Comparator = &ComparatorImpl[*modfile.Exclude, module.Version]{
	GetComparablesFn: func(f *modfile.File) []*modfile.Exclude { return f.Exclude },
	KeyComparableFn:  func(e *modfile.Exclude) module.Version { return e.Mod },
	CompareFn:        func(left, right *modfile.Exclude) bool { return left.Mod == right.Mod },

	CompareNotEqualFmtStr: "XX exclude %s: root(%v) apis(%v)\n",
	FixFn: func(file *modfile.File, toDrop module.Version, toAdd *modfile.Exclude) error {
		if err := file.DropExclude(toDrop.Path, toDrop.Version); err != nil {
			fmt.Printf("Error dropping exclusion for %s: %s\n", toDrop, err)
			return err
		}
		if err := file.AddExclude(toAdd.Mod.Path, toAdd.Mod.Version); err != nil {
			fmt.Printf("Error adding exclusion for %s: %s\n", toDrop, err)
			return err
		}
		return nil
	},
}

var ReplaceComparator = &ComparatorImpl[*modfile.Replace, module.Version]{
	GetComparablesFn: func(f *modfile.File) []*modfile.Replace { return f.Replace },
	KeyComparableFn:  func(r *modfile.Replace) module.Version { return r.Old },
	CompareFn:        func(left, right *modfile.Replace) bool { return left.Old == right.Old && left.New == right.New },

	CompareNotEqualFmtStr: "XX replace %s: root(%v) apis(%v)\n",
	FixFn: func(file *modfile.File, toDrop module.Version, toAdd *modfile.Replace) error {
		if err := file.DropReplace(toDrop.Path, toDrop.Version); err != nil {
			fmt.Printf("Error dropping replacement for %v: %s\n", toDrop, err)
			return err
		}
		if err := file.AddReplace(toAdd.Old.Path, toAdd.Old.Version, toAdd.New.Path, toAdd.New.Version); err != nil {
			fmt.Printf("Error adding replacement for %s: %s\n", toDrop, err)
			return err
		}
		return nil
	},
}

package main

import (
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

func main() {
	rootgmf := readGoMod("go.mod")
	apisgmf := readGoMod("apis/go.mod")
	insync := true
	insync = cmp1("require", mapRequire(rootgmf.Require), mapRequire(apisgmf.Require)) && insync
	insync = cmp1("exclude", mapExclude(rootgmf.Exclude), mapExclude(apisgmf.Exclude)) && insync
	insync = cmp2("replace", mapReplace(rootgmf.Replace), mapReplace(apisgmf.Replace)) && insync
	if insync {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

func readGoMod(path string) *modfile.File {
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

// TODO: Figure out how to collapse these with generics
// LATER: Virtually impossible. My attempts ultimately didn't and couldn't work, and were more
// LOC anyway. Sad face.
func mapRequire(theList []*modfile.Require) map[string]string {
	ret := make(map[string]string, len(theList))
	for _, item := range theList {
		path, ver := (*item).Mod.Path, item.Mod.Version
		if existingver, ok := ret[path]; ok {
			fmt.Printf("WARNING: require path %s listed at multiple versions: %s | %s\n", path, existingver, ver)
		}
		ret[path] = ver
	}
	return ret
}
func mapExclude(theList []*modfile.Exclude) map[string]string {
	ret := make(map[string]string, len(theList))
	for _, item := range theList {
		path, ver := item.Mod.Path, item.Mod.Version
		if existingver, ok := ret[path]; ok {
			fmt.Printf("WARNING: exclude path %s listed at multiple versions: %s | %s\n", path, existingver, ver)
		}
		ret[path] = ver
	}
	return ret
}

type replacement struct {
	oldver  string
	newpath string
	newver  string
}

func mapReplace(theList []*modfile.Replace) map[string]replacement {
	ret := make(map[string]replacement, len(theList))
	for _, item := range theList {
		path := item.Old.Path
		repl := replacement{
			oldver:  item.Old.Version,
			newpath: item.New.Path,
			newver:  item.New.Version,
		}
		if existingrepl, ok := ret[path]; ok {
			fmt.Printf("WARNING: replace path %s listed more than once:\n\t%#v\n\t%#v\n", path, existingrepl, repl)
		}
		ret[path] = repl
	}
	return ret
}

func cmp1(category string, root, apis map[string]string) bool {
	insync := true
	for path, rootver := range root {
		apisver, ok := apis[path]
		if !ok {
			// For a future "verbose mode":
			// fmt.Printf("\t(path in root but not apis: %s)\n", path)
			continue
		}
		if rootver == apisver {
			// For a future "verbose mode":
			// fmt.Printf("\tOK %s %s\n", path, rootver)
		} else {
			fmt.Printf("XX %s %s: root(%s) apis(%s)\n", category, path, rootver, apisver)
			insync = false
		}
	}
	return insync
}

func cmp2(category string, root, apis map[string]replacement) bool {
	insync := true
	for path, rootrepl := range root {
		apisrepl, ok := apis[path]
		if !ok {
			// For a future "verbose mode":
			// fmt.Printf("\t(path in root but not apis: %s)\n", path)
			continue
		}
		if rootrepl == apisrepl {
			// For a future "verbose mode":
			// fmt.Printf("\tOK %s %s\n", path, rootrepl)
		} else {
			fmt.Printf("XX %s %s: root(%s) apis(%s)\n", category, path, rootrepl, apisrepl)
			insync = false
		}
	}
	return insync
}

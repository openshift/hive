package main

import (
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

const (
	rootpath = "go.mod"
	apispath = "apis/go.mod"
)

// TODO: un-global
var doFix bool

func main() {
	// TODO: Actual arg parsing
	if len(os.Args) > 1 && os.Args[1] == "-f" {
		doFix = true
	}
	rootgmf := readGoMod(rootpath)
	apisgmf := readGoMod(apispath)
	needWrite := false
	insync, err := processRequire(*apisgmf, mapRequire(rootgmf.Require), mapRequire(apisgmf.Require))
	if err != nil {
		// processRequire() printed the error
		os.Exit(2)
	}
	if doFix && !insync {
		needWrite = true
		// *Now* the files are in sync. This informs the exit code, which should be "success"
		// if we fully fixed the file. (May still be "failure" if other mismatches are found.)
		insync = true
	}

	// TODO: Make these respond to doFix
	insync = cmpExclude(mapExclude(rootgmf.Exclude), mapExclude(apisgmf.Exclude)) && insync
	insync = cmpReplace(mapReplace(rootgmf.Replace), mapReplace(apisgmf.Replace)) && insync

	if needWrite {
		fmt.Printf("Writing modified %s\n", apispath)
		apisgmf.Cleanup()
		b, err := apisgmf.Format()
		if err != nil {
			fmt.Printf("Couldn't format modified %s: %s\n", apispath, err)
			os.Exit(2)
		}
		if err = os.WriteFile(apispath, b, 0); err != nil {
			fmt.Printf("Failed to write modified %s: %s\n", apispath, err)
			os.Exit(2)
		}
		fmt.Printf("\tDone\n")
	}

	if insync {
		fmt.Printf("%s is in sync\n", apispath)
		if needWrite {
			fmt.Printf("\t(after fixing)\n")
		}
		os.Exit(0)
	} else {
		fmt.Printf("\n%s is out of sync\n", apispath)
		if needWrite {
			fmt.Printf("\t(despite partial fixing)\n")
		}
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
	newpath string
	newver  string
}

func mapReplace(theList []*modfile.Replace) map[string]replacement {
	ret := make(map[string]replacement, len(theList))
	for _, item := range theList {
		// If the replaced component (on the left) has a version specified explicitly, include it in the hash.
		// This allows us to specify replacements for different versions of the same library.
		path := item.Old.Path
		if item.Old.Version != "" {
			path = path + "@" + item.Old.Version
		}
		repl := replacement{
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

func processRequire(apisfile modfile.File, root, apis map[string]string) (bool, error) {
	// insync indicates whether the require versions were in sync *to start*. I.e. if fixing,
	// false indicates that we fixed something (so the files are *now* in sync, pending write).
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
			fmt.Printf("XX require %s: root(%s) apis(%s)\n", path, rootver, apisver)
			insync = false
			if doFix {
				if err := apisfile.DropRequire(path); err != nil {
					fmt.Printf("Error dropping requirement for %s: %s\n", path, err)
					return false, err
				}
				if err := apisfile.AddRequire(path, rootver); err != nil {
					fmt.Printf("Error adding requirement for %s: %s\n", path, err)
					return false, err
				}
				fmt.Printf("\tFixed\n")
			}
		}
	}
	return insync, nil
}

func cmpExclude(root, apis map[string]string) bool {
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
			fmt.Printf("XX exclude %s: root(%s) apis(%s)\n", path, rootver, apisver)
			insync = false
		}
	}
	return insync
}

func cmpReplace(root, apis map[string]replacement) bool {
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
			fmt.Printf("XX replace %s: root(%s) apis(%s)\n", path, rootrepl, apisrepl)
			insync = false
		}
	}
	return insync
}

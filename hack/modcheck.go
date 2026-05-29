package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

const (
	rootpath = "go.mod"
	apispath = "apis/go.mod"
)

type options struct {
	fix            bool
	gomod          string
	allowDowngrade bool
}

func parseArgs() *options {
	opts := &options{}
	flag.BoolVar(&opts.fix, "fix", false, "Fix mismatches by updating the target go.mod")
	flag.StringVar(&opts.gomod, "gomod", apispath, "Path to go.mod file to check/fix")
	flag.BoolVar(&opts.allowDowngrade, "allow-downgrade", false, "Allow fixing mismatches that would downgrade versions (only valid with --fix)")
	flag.Parse()
	return opts
}

func (o *options) validate() error {
	if _, err := os.Stat(o.gomod); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("specified go.mod file does not exist: %s", o.gomod)
		}
		return fmt.Errorf("cannot access go.mod file %s: %v", o.gomod, err)
	}
	if o.allowDowngrade && !o.fix {
		return fmt.Errorf("--allow-downgrade is only valid with --fix")
	}
	return nil
}

func main() {
	opts := parseArgs()

	if err := opts.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	rootgmf := readGoMod(rootpath)
	targetgmf := readGoMod(opts.gomod)

	allInSync, madeChanges, err := opts.processRequire(*targetgmf, mapRequire(rootgmf.Require), mapRequire(targetgmf.Require))
	if err != nil {
		// processRequire() printed the error
		os.Exit(2)
	}

	// TODO: Make these respond to opts.fix
	allInSync = cmpExclude(mapExclude(rootgmf.Exclude), mapExclude(targetgmf.Exclude)) && allInSync
	allInSync = cmpReplace(mapReplace(rootgmf.Replace), mapReplace(targetgmf.Replace)) && allInSync

	if madeChanges {
		fmt.Printf("Writing modified %s\n", opts.gomod)
		targetgmf.Cleanup()
		b, err := targetgmf.Format()
		if err != nil {
			fmt.Printf("Couldn't format modified %s: %s\n", opts.gomod, err)
			os.Exit(2)
		}
		if err = os.WriteFile(opts.gomod, b, 0); err != nil {
			fmt.Printf("Failed to write modified %s: %s\n", opts.gomod, err)
			os.Exit(2)
		}
		fmt.Printf("\tDone\n")
	}

	if allInSync {
		fmt.Printf("%s is in sync\n", opts.gomod)
		if madeChanges {
			fmt.Printf("\t(after fixing)\n")
		}
		os.Exit(0)
	} else {
		fmt.Printf("\n%s is out of sync\n", opts.gomod)
		if madeChanges {
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

// compareVersions returns:
//   - negative if v1 < v2 (v2 is newer)
//   - 0 if v1 == v2
//   - positive if v1 > v2 (v1 is newer)
//
// Also returns a visual indicator: ">>" for upgrade (target behind), "<<" for downgrade (target ahead), "==" for same
func compareVersions(v1, v2 string) (int, string) {
	// semver.Compare requires "v" prefix
	sv1 := v1
	if sv1 != "" && sv1[0] != 'v' {
		sv1 = "v" + sv1
	}
	sv2 := v2
	if sv2 != "" && sv2[0] != 'v' {
		sv2 = "v" + sv2
	}

	cmp := semver.Compare(sv1, sv2)
	var indicator string
	switch {
	case cmp < 0:
		indicator = ">>" // target is behind, would upgrade to root
	case cmp > 0:
		indicator = "<<" // target is ahead, would downgrade to root
	default:
		indicator = "=="
	}
	return cmp, indicator
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

func (o *options) processRequire(targetfile modfile.File, root, target map[string]string) (bool, bool, error) {
	// Returns: (allInSync, madeChanges, error)
	// - allInSync: true if all deps are in sync (either were already, or we fixed them)
	// - madeChanges: true if we made any changes that need to be written
	allInSync := true
	madeChanges := false
	for path, rootver := range root {
		targetver, ok := target[path]
		if !ok {
			// For a future "verbose mode":
			// fmt.Printf("\t(path in root but not target: %s)\n", path)
			continue
		}
		if rootver == targetver {
			// For a future "verbose mode":
			// fmt.Printf("\tOK %s %s\n", path, rootver)
			continue
		}
		// Found a mismatch
		cmp, indicator := compareVersions(targetver, rootver)
		fmt.Printf("%s require %s: root(%s) target(%s)\n", indicator, path, rootver, targetver)

		if !o.fix {
			allInSync = false
			continue
		}

		// We're in fix mode
		// If this would be a downgrade and downgrades aren't allowed, refuse
		if cmp > 0 && !o.allowDowngrade {
			fmt.Printf("\tRefused: would downgrade from %s to %s (use --allow-downgrade to override)\n", targetver, rootver)
			allInSync = false // couldn't fix this one
			continue
		}

		// Fix it
		if err := targetfile.DropRequire(path); err != nil {
			fmt.Printf("Error dropping requirement for %s: %s\n", path, err)
			return false, false, err
		}
		if err := targetfile.AddRequire(path, rootver); err != nil {
			fmt.Printf("Error adding requirement for %s: %s\n", path, err)
			return false, false, err
		}
		madeChanges = true
		if cmp > 0 {
			fmt.Printf("\tFixed (Downgraded)\n")
		} else {
			fmt.Printf("\tFixed\n")
		}
		// This one is now in sync, so don't set allInSync = false
	}
	return allInSync, madeChanges, nil
}

func cmpExclude(root, target map[string]string) bool {
	insync := true
	for path, rootver := range root {
		targetver, ok := target[path]
		if !ok {
			// For a future "verbose mode":
			// fmt.Printf("\t(path in root but not target: %s)\n", path)
			continue
		}
		if rootver == targetver {
			// For a future "verbose mode":
			// fmt.Printf("\tOK %s %s\n", path, rootver)
		} else {
			_, indicator := compareVersions(targetver, rootver)
			fmt.Printf("%s exclude %s: root(%s) target(%s)\n", indicator, path, rootver, targetver)
			insync = false
		}
	}
	return insync
}

func cmpReplace(root, target map[string]replacement) bool {
	insync := true
	for path, rootrepl := range root {
		targetrepl, ok := target[path]
		if !ok {
			// For a future "verbose mode":
			// fmt.Printf("\t(path in root but not target: %s)\n", path)
			continue
		}
		if rootrepl == targetrepl {
			// For a future "verbose mode":
			// fmt.Printf("\tOK %s %s\n", path, rootrepl)
		} else {
			// For replace, compare the new versions
			_, indicator := compareVersions(targetrepl.newver, rootrepl.newver)
			fmt.Printf("%s replace %s: root(%s) target(%s)\n", indicator, path, rootrepl, targetrepl)
			insync = false
		}
	}
	return insync
}

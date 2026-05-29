package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

const (
	rootpath = "go.mod"
	apispath = "apis/go.mod"
)

type options struct {
	fix   bool
	gomod string
}

func parseArgs() *options {
	opts := &options{}
	flag.BoolVar(&opts.fix, "fix", false, "Fix mismatches by updating the target go.mod")
	flag.StringVar(&opts.gomod, "gomod", apispath, "Path to go.mod file to check/fix")
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
	needWrite := false
	insync, err := opts.processRequire(*targetgmf, mapRequire(rootgmf.Require), mapRequire(targetgmf.Require))
	if err != nil {
		// processRequire() printed the error
		os.Exit(2)
	}
	if opts.fix && !insync {
		needWrite = true
		// *Now* the files are in sync. This informs the exit code, which should be "success"
		// if we fully fixed the file. (May still be "failure" if other mismatches are found.)
		insync = true
	}

	// TODO: Make these respond to opts.fix
	insync = cmpExclude(mapExclude(rootgmf.Exclude), mapExclude(targetgmf.Exclude)) && insync
	insync = cmpReplace(mapReplace(rootgmf.Replace), mapReplace(targetgmf.Replace)) && insync

	if needWrite {
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

	if insync {
		fmt.Printf("%s is in sync\n", opts.gomod)
		if needWrite {
			fmt.Printf("\t(after fixing)\n")
		}
		os.Exit(0)
	} else {
		fmt.Printf("\n%s is out of sync\n", opts.gomod)
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

func (o *options) processRequire(targetfile modfile.File, root, target map[string]string) (bool, error) {
	// insync indicates whether the require versions were in sync *to start*. I.e. if fixing,
	// false indicates that we fixed something (so the files are *now* in sync, pending write).
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
			fmt.Printf("XX require %s: root(%s) target(%s)\n", path, rootver, targetver)
			insync = false
			if o.fix {
				if err := targetfile.DropRequire(path); err != nil {
					fmt.Printf("Error dropping requirement for %s: %s\n", path, err)
					return false, err
				}
				if err := targetfile.AddRequire(path, rootver); err != nil {
					fmt.Printf("Error adding requirement for %s: %s\n", path, err)
					return false, err
				}
				fmt.Printf("\tFixed\n")
			}
		}
	}
	return insync, nil
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
			fmt.Printf("XX exclude %s: root(%s) target(%s)\n", path, rootver, targetver)
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
			fmt.Printf("XX replace %s: root(%s) target(%s)\n", path, rootrepl, targetrepl)
			insync = false
		}
	}
	return insync
}

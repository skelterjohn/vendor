/*
Copyright 2015 Google Inc. All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skelterjohn/vendor/vend"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: vendor [-d DIR] -s [-a PATH=REPO]* CONFIG  # save\n")
	fmt.Fprintf(os.Stderr, "       vendor [-d DIR] -r CONFIG                  # restore\n")
	os.Exit(1)
}

func main() {
	fs := flag.NewFlagSet("vendor", flag.ExitOnError)
	dir := fs.String("d", ".", "directory to vendor into")
	save := fs.Bool("s", false, "save the repos and revisions")
	restore := fs.Bool("r", false, "restore the repos and revisions")
	version := fs.Bool("v", false, "print vendor version info")
	extend := fs.Bool("x", false, "extend the existing config instead of overwriting it")

	var args, addons, rgits, rhgs []string
	isMaybeSave := false
	var isAddon, isGit, isHg bool
	for _, arg := range os.Args[1:] {
		if isAddon {
			addons = append(addons, arg)
			isAddon = false
			continue
		}
		if isGit {
			rgits = append(rgits, arg)
			isGit = false
			continue
		}
		if isHg {
			rhgs = append(rhgs, arg)
			isHg = false
			continue
		}
		switch arg {
		case "-s":
			isMaybeSave = true
			args = append(args, arg)
		case "-a":
			if !isMaybeSave {
				args = append(args, arg)
			} else {
				isAddon = true
			}
		case "-rgit":
			if !isMaybeSave {
				args = append(args, arg)
			} else {
				isGit = true
			}
		case "-rhg":
			if !isMaybeSave {
				args = append(args, arg)
			} else {
				isHg = true
			}
		default:
			args = append(args, arg)
		}

	}

	ignored := map[string]bool{}
	ignoredStr := os.Getenv("VENDOR_IGNORE_DIRS")
	for _, ignore := range strings.Split(ignoredStr, string(filepath.ListSeparator)) {
		ignored[ignore] = true
	}

	fs.Parse(args)

	if *version {
		fmt.Println("vendor build 3")
		os.Exit(0)
	}

	if fs.NArg() != 1 || *save == *restore {
		usage()
	}
	var err error
	if *save {
		err = vend.Save(*dir, fs.Arg(0), addons, rgits, rhgs, ignored, *extend)
	}
	if *restore {
		err = vend.Restore(*dir, fs.Arg(0))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

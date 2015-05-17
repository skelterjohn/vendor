package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Config struct {
	GitRepos       map[string]GitRepo
	MercurialRepos map[string]HgRepo
	sync.Mutex
}

type GitRepo struct {
	URI string
	Ref string
}

type HgRepo struct {
	URI string
	Ref string
}

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

	if *save {
		doSave(*dir, fs.Arg(0), addons, rgits, rhgs, ignored, *extend)
	}
	if *restore {
		doRestore(*dir, fs.Arg(0))
	}
}

func orExit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func saveRepo(wg *sync.WaitGroup, cfg, oldCfg *Config, path string, repoPath string) error {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		wg.Add(1)
		go func() {
			if err := saveGit(cfg, oldCfg, path, repoPath); err != nil {
				fmt.Fprintf(os.Stderr, "%q: %s\n", path, err)
			}
			wg.Done()
		}()
		return filepath.SkipDir
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".hg")); err == nil {
		wg.Add(1)
		go func() {
			if err := saveMercurial(cfg, oldCfg, path, repoPath); err != nil {
				fmt.Fprintf(os.Stderr, "%q: %s\n", path, err)
			}
			wg.Done()
		}()
		return filepath.SkipDir
	}
	return nil
}

func doSave(dir, cfgPath string, addons, rgits, rhgs []string, ignored map[string]bool, extend bool) {
	cfg := Config{
		GitRepos:       map[string]GitRepo{},
		MercurialRepos: map[string]HgRepo{},
	}

	var oldCfg Config
	if in, err := os.Open(cfgPath); err == nil {
		_ = json.NewDecoder(in).Decode(&oldCfg)
	}

	var wg sync.WaitGroup

	scanDir := func(path string, info os.FileInfo, err error) error {
		// don't vendor the root, that'd be pointless
		if path == "." {
			return nil
		}
		if ignored[path] {
			return filepath.SkipDir
		}
		if err := saveRepo(&wg, &cfg, &oldCfg, path, path); err != nil {
			return err
		}
		return nil
	}
	orExit(filepath.Walk(dir, scanDir))

	for _, addon := range addons {
		tokens := strings.Split(addon, "=")
		path, repoPath := tokens[0], tokens[1]
		saveRepo(&wg, &cfg, &oldCfg, path, repoPath)
	}

	wg.Wait()

	for _, rgit := range rgits {
		tokens := strings.Split(rgit, "=")
		path, reporev := tokens[0], tokens[1]
		tokens = strings.Split(reporev, "@")
		repo, rev := tokens[0], tokens[1]
		cfg.GitRepos[path] = GitRepo{
			URI: repo,
			Ref: rev,
		}
	}

	for _, rhg := range rhgs {
		tokens := strings.Split(rhg, "=")
		path, reporev := tokens[0], tokens[1]
		tokens = strings.Split(reporev, "@")
		repo, rev := tokens[0], tokens[1]
		cfg.MercurialRepos[path] = HgRepo{
			URI: repo,
			Ref: rev,
		}
	}

	for path, newRepo := range cfg.GitRepos {
		if oldRepo, ok := oldCfg.GitRepos[path]; !ok || newRepo != oldRepo {
			fmt.Println(path)
			if oldRepo.Ref != "" {
				fmt.Fprintf(os.Stderr, "%s -> %s\n", oldRepo.Ref, newRepo.Ref)
			}
		}
	}
	for path, newRepo := range cfg.MercurialRepos {
		if oldRepo, ok := oldCfg.MercurialRepos[path]; !ok || newRepo != oldRepo {
			fmt.Println(path)
			if oldRepo.Ref != "" {
				fmt.Fprintf(os.Stderr, "%s -> %s\n", oldRepo.Ref, newRepo.Ref)
			}
		}
	}

	if extend {
		if oldCfg.GitRepos == nil {
			oldCfg.GitRepos = map[string]GitRepo{}
		}
		if oldCfg.MercurialRepos == nil {
			oldCfg.MercurialRepos = map[string]HgRepo{}
		}
		for path, newRepo := range cfg.GitRepos {
			oldCfg.GitRepos[path] = newRepo
		}
		for path, newRepo := range cfg.MercurialRepos {
			oldCfg.MercurialRepos[path] = newRepo
		}
		cfg = oldCfg
	}

	out, err := os.Create(cfgPath)
	orExit(err)
	var buf bytes.Buffer
	orExit(json.NewEncoder(&buf).Encode(&cfg))
	var indented bytes.Buffer
	orExit(json.Indent(&indented, buf.Bytes(), "", "  "))
	fmt.Fprint(out, &indented)
}

func doRestore(dir, cfgPath string) {
	in, err := os.Open(cfgPath)
	orExit(err)
	var cfg Config
	orExit(json.NewDecoder(in).Decode(&cfg))

	wg := sync.WaitGroup{}

	for path, gitRepo := range cfg.GitRepos {
		wg.Add(1)
		go func(path string, gitRepo GitRepo) {
			repoPath := filepath.Join(dir, path)
			if restoreGit(repoPath, gitRepo) {
				fmt.Fprint(os.Stdout, path+"\n")
			}
			wg.Done()
		}(path, gitRepo)
	}

	for path, hgRepo := range cfg.MercurialRepos {
		wg.Add(1)
		go func(path string, hgRepo HgRepo) {
			repoPath := filepath.Join(dir, path)
			if restoreMercurial(repoPath, hgRepo) {
				fmt.Fprint(os.Stdout, path+"\n")
			}
			wg.Done()
		}(path, hgRepo)
	}

	wg.Wait()
}

func restoreGit(path string, repo GitRepo) bool {
	resetHard := func(printErr bool) bool {
		cmd := exec.Command("git", "reset", "--hard", repo.Ref)
		cmd.Dir = path
		cmd.Stdout = ioutil.Discard
		var errBuf bytes.Buffer
		if printErr {
			cmd.Stderr = &errBuf
		} else {
			cmd.Stderr = ioutil.Discard
		}
		if cmd.Run() != nil {
			if printErr {
				os.Stderr.Write(errBuf.Bytes())
			}
			return false
		}
		return true
	}

	// check if it's up to date
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Stderr = os.Stderr
	cmd.Dir = path
	if output, err := cmd.Output(); err == nil {
		ref := strings.TrimSpace(string(output))
		if repo.Ref == ref {
			return false
		}
		if resetHard(false) {
			return true
		}
	}

	os.RemoveAll(path)

	var errBuf bytes.Buffer
	fmt.Fprintf(&errBuf, "restoring %q:\n", path)
	cmd = exec.Command("git", "clone", repo.URI, path)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &errBuf
	if cmd.Run() != nil {
		os.Stderr.Write(errBuf.Bytes())
		return false
	}

	return resetHard(true)
}

func saveGit(cfg, oldCfg *Config, path, repoPath string) error {
	//git rev-parse HEAD
	gr := GitRepo{}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Stderr = os.Stderr
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err != nil {
		return err
	} else {
		gr.Ref = strings.TrimSpace(string(output))
	}
	cmd = exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Stderr = os.Stderr
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err != nil {
		return err
	} else {
		gr.URI = strings.TrimSpace(string(output))
	}

	cfg.Lock()
	cfg.GitRepos[path] = gr
	cfg.Unlock()

	return nil
}

func restoreMercurial(path string, repo HgRepo) bool {
	// check if it's up to date
	cmd := exec.Command("hg", "id", "-i")
	cmd.Stderr = os.Stderr
	cmd.Dir = path
	if output, err := cmd.Output(); err == nil {
		ref := strings.TrimSpace(string(output))
		if repo.Ref == ref {
			return false
		}
		cmd := exec.Command("hg", "update", repo.Ref)
		cmd.Dir = path
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = ioutil.Discard
		if cmd.Run() == nil {
			return true
		}
	}

	os.RemoveAll(path)

	var errBuf bytes.Buffer
	fmt.Fprintf(&errBuf, "restoring %q:\n", path)
	cmd = exec.Command("hg", "clone", repo.URI, "-u", repo.Ref, path)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &errBuf
	if cmd.Run() != nil {
		os.Stderr.Write(errBuf.Bytes())
		return false
	}

	return true
}

func saveMercurial(cfg, oldCfg *Config, path, repoPath string) error {
	//git rev-parse HEAD
	hr := HgRepo{}
	cmd := exec.Command("hg", "--debug", "id", "-i")
	cmd.Stderr = os.Stderr
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err != nil {
		return err
	} else {
		hr.Ref = strings.TrimSpace(string(output))
	}
	cmd = exec.Command("hg", "paths", "default")
	cmd.Stderr = os.Stderr
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err != nil {
		return err
	} else {
		hr.URI = strings.TrimSpace(string(output))
	}

	cfg.Lock()
	cfg.MercurialRepos[path] = hr
	cfg.Unlock()

	return nil
}

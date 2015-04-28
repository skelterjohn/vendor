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
	GitRepos map[string]GitRepo
}

type GitRepo struct {
	URI string
	Ref string
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: vendor [-d DIR] -s [-a REPO=PATH]* CONFIG  # save\n")
	fmt.Fprintf(os.Stderr, "       vendor [-d DIR] -r CONFIG                  # restore\n")
	os.Exit(1)
}

func main() {
	fs := flag.NewFlagSet("vendor", flag.ExitOnError)
	dir := fs.String("d", ".", "directory to vendor into")
	save := fs.Bool("s", false, "save the repos and revisions")
	restore := fs.Bool("r", false, "restore the repos and revisions")

	var args, addons []string
	isMaybeSave := false
	isAddon := false
	for _, arg := range os.Args[1:] {
		if isAddon {
			addons = append(addons, arg)
			isAddon = false
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
		default:
			args = append(args, arg)
		}

	}

	fs.Parse(args)
	if fs.NArg() != 1 || *save == *restore {
		usage()
	}

	if *save {
		doSave(*dir, fs.Arg(0), addons)
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

func saveRepo(cfg *Config, path string, repoPath string) error {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		return err
	}
	if gr, err := saveGit(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "%q: %s\n", path, err)
		return err
	} else {
		cfg.GitRepos[path] = gr
		fmt.Println(path)
	}
	return nil
}

func doSave(dir, cfgPath string, addons []string) {
	cfg := Config{
		GitRepos: map[string]GitRepo{},
	}

	scanDir := func(path string, info os.FileInfo, err error) error {
		// don't vendor the root, that'd be pointless
		if path == "." {
			return nil
		}
		if err := saveRepo(&cfg, path, path); err == nil {
			return filepath.SkipDir
		}
		return nil
	}
	orExit(filepath.Walk(dir, scanDir))

	for _, addon := range addons {
		tokens := strings.Split(addon, "=")
		path, repoPath := tokens[0], tokens[1]
		saveRepo(&cfg, path, repoPath)
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

	for path := range cfg.GitRepos {
		repoPath := filepath.Join(dir, path)
		os.RemoveAll(repoPath)
	}
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

	wg.Wait()
}

func restoreGit(path string, repo GitRepo) bool {
	var errBuf bytes.Buffer
	fmt.Fprintf(&errBuf, "restoring %q:\n", path)
	cmd := exec.Command("git", "clone", repo.URI, path)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &errBuf
	if cmd.Run() != nil {
		os.Stderr.Write(errBuf.Bytes())
		return false
	}

	cmd = exec.Command("git", "reset", "--hard", repo.Ref)
	cmd.Dir = path
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &errBuf
	if cmd.Run() != nil {
		os.Stderr.Write(errBuf.Bytes())
		return false
	}

	return true
}

func saveGit(path string) (GitRepo, error) {
	//git rev-parse HEAD
	gr := GitRepo{}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Stderr = os.Stderr
	cmd.Dir = path
	if output, err := cmd.Output(); err != nil {
		return gr, err
	} else {
		gr.Ref = strings.TrimSpace(string(output))
	}
	cmd = exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Stderr = os.Stderr
	cmd.Dir = path
	if output, err := cmd.Output(); err != nil {
		return gr, err
	} else {
		gr.URI = strings.TrimSpace(string(output))
	}
	return gr, nil
}

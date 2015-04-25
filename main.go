package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	fmt.Fprintf(os.Stderr, "Usage: vendor [-d DIR] -s -|CONFIG  # save\n")
	fmt.Fprintf(os.Stderr, "       vendor [-d DIR] -r -|CONFIG  # restore\n")
	os.Exit(1)
}

func main() {
	dir := flag.String("d", ".", "directory to vendor into")
	save := flag.Bool("s", false, "save the repos and revisions")
	restore := flag.Bool("r", false, "restore the repos and revisions")
	flag.Parse()
	if flag.NArg() != 1 || *save == *restore {
		usage()
	}

	if *save {
		doSave(*dir, flag.Arg(0))
	}
	if *restore {
		doRestore(*dir, flag.Arg(0))
	}
}

func orExit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func doSave(dir, cfgPath string) {
	cfg := Config{
		GitRepos: map[string]GitRepo{},
	}

	scanDir := func(path string, info os.FileInfo, err error) error {
		if _, serr := os.Stat(filepath.Join(path, ".git")); serr == nil {
			if gr, err := saveGit(path); err != nil {
				fmt.Fprintf(os.Stderr, "%q: %s\n", path, err)
			} else {
				cfg.GitRepos[path] = gr
			}
			return filepath.SkipDir
		}
		return nil
	}
	orExit(filepath.Walk(dir, scanDir))

	var out io.Writer
	if cfgPath == "-" {
		out = os.Stdout
	} else {
		var err error
		out, err = os.Create(cfgPath)
		orExit(err)
	}
	var buf bytes.Buffer
	orExit(json.NewEncoder(&buf).Encode(&cfg))
	var indented bytes.Buffer
	orExit(json.Indent(&indented, buf.Bytes(), "", "  "))
	fmt.Fprint(out, &indented)
}

func doRestore(dir, cfgPath string) {
	var in io.Reader
	if cfgPath == "-" {
		in = os.Stdin
	} else {
		var err error
		in, err = os.Open(cfgPath)
		orExit(err)
	}
	var cfg Config
	orExit(json.NewDecoder(in).Decode(&cfg))

	wg := sync.WaitGroup{}

	for path, gitRepo := range cfg.GitRepos {
		repoPath := filepath.Join(dir, path)
		wg.Add(1)
		go func() {
			restoreGit(repoPath, gitRepo)
			wg.Done()
		}()
	}

	wg.Wait()
}

func restoreGit(path string, repo GitRepo) {
	var errBuf bytes.Buffer
	fmt.Fprintf(&errBuf, "restoring %q:\n", path)
	cmd := exec.Command("git", "clone", repo.URI, path)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &errBuf
	if cmd.Run() != nil {
		os.Stderr.Write(errBuf.Bytes())
		return
	}

	cmd = exec.Command("git", "reset", "--hard", repo.Ref)
	cmd.Dir = path
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = &errBuf
	if cmd.Run() != nil {
		os.Stderr.Write(errBuf.Bytes())
		return
	}
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

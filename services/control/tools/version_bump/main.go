package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

func replaceOne(path string, re *regexp.Regexp, replacement string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	src := string(raw)
	loc := re.FindStringIndex(src)
	if loc == nil {
		return false, fmt.Errorf("pattern not found in %s", path)
	}
	if re.FindStringIndex(src[loc[1]:]) != nil {
		return false, fmt.Errorf("pattern occurs more than once in %s", path)
	}
	out := re.ReplaceAllString(src, replacement)
	if out == src {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func replaceOptional(path string, re *regexp.Regexp, replacement string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if !re.Match(raw) {
		return false, nil
	}
	return replaceOne(path, re, replacement)
}

func main() {
	version := flag.String("version", "", "version string (e.g. 1.0.1)")
	flag.Parse()
	if *version == "" {
		fmt.Fprintln(os.Stderr, "missing --version")
		os.Exit(2)
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	controlRoot := wd

	controlBuildInfo := filepath.Join(controlRoot, "internal", "buildinfo", "buildinfo.go")
	nodeConfig := filepath.Join(controlRoot, "..", "node", "src", "config.rs")
	nodeCargo := filepath.Join(controlRoot, "..", "node", "Cargo.toml")

	changed := 0
	{
		// Bumping the fallback literal in buildinfo.go is belt-and-suspenders:
		// release builds inject the version via -ldflags, so this fallback is
		// only visible to `go run` / IDE developers. But keeping it fresh
		// means dev builds print sensible version info in logs.
		re := regexp.MustCompile(`(?m)^var appVersion = ".*"$`)
		repl := fmt.Sprintf(`var appVersion = "%s"`, *version)
		ok, err := replaceOne(controlBuildInfo, re, repl)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if ok {
			fmt.Println(controlBuildInfo)
			changed++
		}
	}
	{
		re := regexp.MustCompile(`(?m)^const HARDCODED_NODE_VERSION: &str = ".*";$`)
		repl := fmt.Sprintf(`const HARDCODED_NODE_VERSION: &str = "%s";`, *version)
		ok, err := replaceOptional(nodeConfig, re, repl)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if ok {
			fmt.Println(nodeConfig)
			changed++
		}
	}
	{
		re := regexp.MustCompile(`(?m)^version = ".*"$`)
		repl := fmt.Sprintf(`version = "%s"`, *version)
		ok, err := replaceOne(nodeCargo, re, repl)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if ok {
			fmt.Println(nodeCargo)
			changed++
		}
	}

	if changed == 0 {
		fmt.Println("no changes")
	}
}

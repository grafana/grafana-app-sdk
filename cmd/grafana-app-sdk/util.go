package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const mkDirPerms = 0755

// checkAndMakePath makes a path if it doesn't already exist. If the path already exists but is not a directory,
// it will error.
func checkAndMakePath(path string) error {
	if fi, err := os.Stat(path); err != nil {
		err = os.MkdirAll(path, mkDirPerms)
		if err != nil {
			return err
		}
	} else if !fi.IsDir() {
		// HMM
		return fmt.Errorf("%s is already present and a file", path)
	}
	return nil
}

// writeFile writes a file with the given contents, creating the needed directories in the path if they don't exist
//
//nolint:errcheck,gosec,revive
func writeFile(path string, contents []byte) error {
	if strings.Index(path, "/") > 0 {
		fp := path[:strings.LastIndex(path, "/")]
		if err := checkAndMakePath(fp); err != nil {
			return err
		}
	}
	fmt.Printf(" * Writing file %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(contents)
	return err
}

// writeFileWithOverwriteConfirm wraps a writeFile call with a Y/n overwrite prompt if the file already exists
func writeFileWithOverwriteConfirm(path string, contents []byte) error {
	if _, err := os.Stat(path); err == nil {
		if promptYN(fmt.Sprintf("File '%s' exists, do you want to overwrite it?", path), true) {
			return writeFile(path, contents)
		}
		return nil
	}
	return writeFile(path, contents)
}

//nolint:errcheck,gosec,revive
func writeExecutableFile(path string, contents []byte) error {
	if strings.Index(path, "/") > 0 {
		fp := path[:strings.LastIndex(path, "/")]
		if err := checkAndMakePath(fp); err != nil {
			return err
		}
	}
	fmt.Printf(" * Writing file %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(contents)
	return err
}

// promptYN prompts the user with `prompt`, asking for a y/n (with a default for no answer), and returns true
// if a `y`/`Y` is supplied, or if defaultAnswer is `true` and the user didn't supply an answer
//
//nolint:revive,unparam
func promptYN(prompt string, defaultAnswer bool) bool {
	y := "y"
	n := "n"
	if defaultAnswer {
		y = "Y"
	} else {
		n = "N"
	}
	input := make([]byte, 1)
	for {
		fmt.Printf("%s [%s/%s]: ", prompt, y, n)
		_, err := bufio.NewReader(os.Stdin).Read(input)
		if err != nil {
			panic(err)
		}
		if input[0] == '\n' || input[0] == '\r' {
			return defaultAnswer
		}
		if input[0] == 'y' || input[0] == 'Y' {
			return true
		}
		if input[0] == 'n' || input[0] == 'N' {
			return false
		}
		fmt.Printf("Could not parse input beginning with '%s', please try again:\n", string(input[0]))
	}
}

type gomod struct {
	Module struct {
		Path string `json:"Path"`
	} `json:"Module"`
}

// getGoModule returns the go module name by running `go mod edit --json` in the directory of the go.mod file
// Linter doesn't like "Potential file inclusion via variable", which is actually desired here
//
//nolint:errcheck,gosec
func getGoModule(goModPath string) (string, error) {
	dir := filepath.Dir(goModPath)

	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("unable to run go mod edit --json: %w", err)
	}

	var mod gomod
	if err := json.Unmarshal(out, &mod); err == nil {
		return mod.Module.Path, nil
	}

	return "", errors.New("unable to locate module in go.mod file")
}

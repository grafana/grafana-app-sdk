package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:  "version",
	RunE: getVersion,
}

const (
	versionOutputTemplate        = "%s\n"
	versionOutputTemplateVerbose = "Version:  %s\nSource:   %s\nCommit:   %s\nBuilt at: %s\n"
)

// These get populated by a `make build` or by the goreleaser
var (
	version = ""
	commit  = "none"
	date    = "unknown"
	// source is only populated by `make build` (goreleaser doesn't put anything here),
	// So we default it to "release binary," as `make build` overwrites it, and so does a go install
	source = "release binary"
)

func setupVersionCmd() {
	versionCmd.Flags().BoolP("verbose", "v", false, "verbose output")
}

//nolint:revive
func getVersion(cmd *cobra.Command, args []string) error {
	if version == "" {
		// If this was installed via `go install`, we can get the version info from debug
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
			source = "go install"
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
				}
				if s.Key == "vcs.time" {
					date = s.Value
				}
			}
		} else {
			version = "dev"
			source = "unknown"
		}
	}
	if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		fmt.Printf(versionOutputTemplateVerbose, version, source, commit, date)
	} else {
		fmt.Printf(versionOutputTemplate, version)
	}
	return nil
}

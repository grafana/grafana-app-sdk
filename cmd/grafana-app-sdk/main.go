package main

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	FormatCUE = "cue"
)

var rootCmd = &cobra.Command{
	Use:   "grafana-app-sdk <command>",
	Short: "A tool for working with grafana apps, used for generating code from CUE kinds, creating project boilerplate, and running local deployments",
	Long:  "A tool for working with grafana apps, used for generating code from CUE kinds, creating project boilerplate, and running local deployments",
}

func main() {
	rootCmd.PersistentFlags().StringP("cuepath", "c", "kinds", "Path to directory with cue.mod")
	rootCmd.PersistentFlags().StringSliceP("selectors", "s", []string{}, "selectors")
	rootCmd.PersistentFlags().StringP("format", "f", "cue", "Format in which kinds are written for this project (currently allowed values are 'cue')")

	setupVersionCmd()
	setupGenerateCmd()
	setupProjectCmd()

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(projectCmd)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

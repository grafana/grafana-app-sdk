package main

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	FormatCUE  = "cue"
	FormatNone = "none"
)

var rootCmd = &cobra.Command{
	Use:   "grafana-app-sdk <command>",
	Short: "A tool for working with grafana apps, used for generating code from CUE kinds, creating project boilerplate, and running local deployments",
	Long:  "A tool for working with grafana apps, used for generating code from CUE kinds, creating project boilerplate, and running local deployments",
}

// Persistent flags for all commands
const (
	sourceFlag           = "source"
	formatFlag           = "format"
	selectorFlag         = "manifest"
	genOperatorStateFlag = "genoperatorstate"
)

func main() {
	rootCmd.PersistentFlags().StringP(sourceFlag, "s", "kinds", "Path to directory with your codegen source files (such as a CUE module)")
	rootCmd.PersistentFlags().StringP(formatFlag, "f", FormatCUE, "Format in which kinds are written for this project (currently allowed values are 'cue')")
	rootCmd.PersistentFlags().String(selectorFlag, "manifest", "Path selector to use for the manifest")
	rootCmd.PersistentFlags().Bool(genOperatorStateFlag, true, "Generate operator state code")

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

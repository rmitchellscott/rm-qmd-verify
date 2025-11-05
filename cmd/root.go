package cmd

import (
	"github.com/spf13/cobra"
	"github.com/rmitchellscott/rm-qmd-verify/internal/version"
)

var rootCmd = &cobra.Command{
	Use:   "qmdverify",
	Short: "QMD verification tool for reMarkable devices",
	Long:  "A tool to verify QMD files against reMarkable device firmware versions",
	Version: version.GetFullVersion(),
	Run: func(cmd *cobra.Command, args []string) {
		runServe(cmd, args)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.SetVersionTemplate("{{.Version}}\n")
}

package cli

import (
	"fmt"
	"github.com/spf13/cobra"
)

var (
    showVersion bool
    version string
)

func init() {
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show CLI version")
}
var rootCmd = &cobra.Command{
	Use:   "alibi",
	Short: "Alibi CLI tool to track project contributions",
	Long:  `Alibi helps track pushes, contributions, and visualize changes in projects.`,

	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			fmt.Printf("alibi version %s\n", version)
			return
		}

		cmd.Help()
	},
}

// Execute is called by main.go
func Execute() error {
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	return rootCmd.Execute()
}

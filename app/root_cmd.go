package app

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "pinger",
}

func AddSubCmd(c ...*cobra.Command) {
	rootCmd.AddCommand(c...)
}

func RootCmd() *cobra.Command {
	return rootCmd
}

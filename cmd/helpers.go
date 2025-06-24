package cmd

import (
	"github.com/spf13/cobra"
)

func isDebug(cmd *cobra.Command) bool {
	if debug, _ := cmd.Flags().GetBool("debug"); debug {
		return true
	}
	return false
}

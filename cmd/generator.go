/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/FMotalleb/junction/internal/cg"
	"github.com/spf13/cobra"
)

// generatorCmd represents the generator command.
var generatorCmd = &cobra.Command{
	Use:   "generator",
	Short: "Simple gui to generate config",
	RunE: func(cmd *cobra.Command, _ []string) error {
		l, err := cmd.Flags().GetString("listen")
		if err != nil {
			return err
		}
		return cg.Serve(l)
	},
}

func init() {
	rootCmd.AddCommand(generatorCmd)
	generatorCmd.Flags().StringP("listen", "l", "127.0.0.1:8080", "listen address, use 0.0.0.0 if you want to publish this globally")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generatorCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// generatorCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

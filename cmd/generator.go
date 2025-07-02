/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/FMotalleb/junction/internal/front"
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
		return front.Serve(l)
	},
}

func init() {
	rootCmd.AddCommand(generatorCmd)
	generatorCmd.Flags().StringP("listen", "l", "127.0.0.1:8080", "listen address, use 0.0.0.0:8080 if you want to publish this globally")
}

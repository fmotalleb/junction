/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/junction/internal/front"
	"github.com/spf13/cobra"
)

// generatorCmd represents the generator command.
var generatorCmd = &cobra.Command{
	Use:   "generator",
	Short: "Simple gui to generate config",
	RunE: func(cmd *cobra.Command, _ []string) error {
		log.SetDebugDefaults()
		l, err := cmd.Flags().GetString("listen")
		if err != nil {
			return err
		}
		ctx, cancel, err := buildAppContext()
		if err != nil {
			return err
		}
		defer cancel()
		return front.Serve(ctx, l)
	},
}

// init registers the generatorCmd as a subcommand of rootCmd and defines the "listen" flag for configuring the listen address.
func init() {
	rootCmd.AddCommand(generatorCmd)
	generatorCmd.Flags().StringP("listen", "l", "127.0.0.1:8080", "listen address, use 0.0.0.0:8080 if you want to publish this globally")
}

/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/fmotalleb/junction/config"
	"github.com/spf13/cobra"
)

// dumpCmd represents the dump command.
var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Reads a config file and dumps it to stdout",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var format, configFile string
		var err error
		var cfg config.Config
		if configFile, err = cmd.Flags().GetString("config"); err != nil {
			fmt.Printf("Error reading config file flag: %v\n", err)
			return err
		}
		if format, err = cmd.Flags().GetString("format"); err != nil {
			fmt.Printf("Error reading format flag: %v\n", err)
			return err
		}
		if err = config.Parse(&cfg, configFile, debug); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			return err
		}
		if err != nil {
			fmt.Println("TOML example has an issue, please report this in issue tracker")
			fmt.Println(string(exampleConfigData))
			return err
		}

		return dumpConf(&cfg, format)
	},
}

func init() {
	rootCmd.AddCommand(dumpCmd)
	dumpCmd.Flags().StringP("config", "c", "", "config file (default: reading config from stdin)")
	dumpCmd.Flags().StringP("format", "f", "toml", "Format of output (only toml file has documents)")
}

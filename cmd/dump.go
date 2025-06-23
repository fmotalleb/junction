/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/FMotalleb/junction/config"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// dumpCmd represents the dump command.
var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Reads a config file and dumps it to stdout",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configFile, format string
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
		if err = config.Parse(&cfg, "", configFile); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			return err
		}
		if err != nil {
			fmt.Println("TOML example has an issue, please report this in issue tracker")
			fmt.Println(string(exampleConfigData))
			return err
		}
		var result []byte
		switch format {
		case "toml":
			result, err = toml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to encode TOML: %w", err)
			}
		case "json":
			result, err = json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to encode JSON: %w", err)
			}
		case "yaml":
			result, err = yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to encode YAML: %w", err)
			}

		}
		if result == nil {
			return fmt.Errorf("given format `%s` is not implemented yet, use one of toml,yaml,json", format)
		}
		fmt.Println(string(result))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dumpCmd)
	dumpCmd.Flags().StringP("config", "c", "", "config file (default: reading config from stdin)")
	dumpCmd.Flags().StringP("format", "f", "toml", "Format of output (only toml file has documents)")
}

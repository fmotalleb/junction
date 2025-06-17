/*
Copyright © 2025 Motalleb Fallahnezhad (fmotalleb@gmail.com)
*/
package cmd

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

//go:embed example/config.toml
var exampleConfigData []byte

// exampleCmd represents the example command.
var exampleCmd = &cobra.Command{
	Use:   "example",
	Short: "An example config file",
	RunE: func(cmd *cobra.Command, _ []string) error {
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		var result []byte
		parsed, err := tomlToMap(exampleConfigData)
		if err != nil {
			fmt.Println("TOML example has an issue, please report this in issue tracker")
			fmt.Println(string(exampleConfigData))
			return err
		}
		switch format {
		case "toml":
			result = exampleConfigData
		case "json":
			result, err = json.MarshalIndent(parsed, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to encode JSON: %w", err)
			}
		case "yaml":
			result, err = yaml.Marshal(parsed)
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
	rootCmd.AddCommand(exampleCmd)
	exampleCmd.Flags().StringP("format", "f", "toml", "Format of output (only toml file has documents)")
}

func tomlToMap(data []byte) (map[string]interface{}, error) {
	tree, err := toml.LoadBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}
	return tree.ToMap(), nil
}

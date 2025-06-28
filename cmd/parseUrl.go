/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/FMotalleb/junction/singbox"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// parseUrlCmd represents the parseUrl command.
var parseUrlCmd = &cobra.Command{
	Use:   "parse-url",
	Short: "Parse a url into singbox outbound",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("parse-url requires one positional argument")
		}

		u, err := url.Parse(args[0])
		if err != nil {
			return errors.Join(
				errors.New("failed to parse given argument to url object"),
				err,
			)
		}
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		var ob map[string]any
		if ob, err = singbox.TryParseOutboundURL(u); err != nil {
			return errors.Join(
				errors.New("failed to parse url into outbound object"),
				err,
			)
		}
		var o []byte
		if o, err = marshalData(ob, format); err != nil {
			return errors.Join(
				errors.New("possible internal issue, failed to marshalize the map"),
				err,
			)
		}
		fmt.Println(string(o))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(parseUrlCmd)
	parseUrlCmd.Flags().StringP("format", "f", "toml", "Format of output")
}

func marshalData(data any, format string) ([]byte, error) {
	switch format {
	case "toml":
		return toml.Marshal(data)
	case "json":
		return json.Marshal(data)
	case "yaml":
		return yaml.Marshal(data)
	}
	return []byte{}, errors.New("unknown format")
}

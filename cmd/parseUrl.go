/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/fmotalleb/junction/services/singbox"
	"github.com/spf13/cobra"
)

var ErrParseURLMissingArg = errors.New("parse-url requires one positional argument")

// parseURLCmd represents the parseUrl command.
var parseURLCmd = &cobra.Command{
	Use:   "parse-url",
	Short: "Parse a url into singbox outbound",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return ErrParseURLMissingArg
		}

		u, err := url.Parse(args[0])
		if err != nil {
			return err
		}
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		var ob map[string]any
		if ob, err = singbox.TryParseOutboundURL(u); err != nil {
			return err
		}
		var o []byte
		if o, err = marshalData(ob, format); err != nil {
			return err
		}
		fmt.Println(string(o))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(parseURLCmd)
	parseURLCmd.Flags().StringP("format", "f", "toml", "Format of output")
}

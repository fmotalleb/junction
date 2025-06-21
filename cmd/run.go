/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"net/url"
	"time"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/server"
	"github.com/spf13/cobra"
)

// runCmd represents the run command.
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a simple server instead of reading a full config file",
	RunE: func(cmd *cobra.Command, _ []string) error {
		entry := new(config.EntryPoint)
		var err error
		if entry.ListenPort, err = cmd.Flags().GetInt("port"); err != nil {
			return err
		}
		var proxyList []string
		if proxyList, err = cmd.Flags().GetStringSlice("proxy"); err != nil {
			return err
		}
		for _, p := range proxyList {
			var pu *url.URL
			if pu, err = url.Parse(p); err != nil {
				return err
			}
			entry.Proxy = append(entry.Proxy, *pu)
		}
		if entry.Routing, err = cmd.Flags().GetString("routing"); err != nil {
			return err
		}
		if entry.Target, err = cmd.Flags().GetString("target"); err != nil {
			return err
		}

		if entry.Timeout, err = cmd.Flags().GetDuration("timeout"); err != nil {
			return err
		}

		var cfg config.Config
		cfg.EntryPoints = []config.EntryPoint{
			*entry,
		}
		if err := server.Serve(cfg); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().IntP("port", "p", 0, "Port to listen on")
	runCmd.Flags().StringSliceP("proxy", "x", nil, "Proxy URLs to forward requests to")
	runCmd.Flags().StringP("routing", "r", "", "Routing rule for the server")
	runCmd.Flags().StringP("target", "t", "", "Target URL to forward requests to")
	runCmd.Flags().DurationP("timeout", "T", time.Hour*24, "Timeout for requests")

	requireOrPanic("port")
	requireOrPanic("target")
	requireOrPanic("routing")
}

func requireOrPanic(name string) {
	if err := runCmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}

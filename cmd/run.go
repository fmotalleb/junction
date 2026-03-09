/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fmotalleb/go-tools/constants"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/server"
)

// runCmd represents the run command.
var runCmd = &cobra.Command{
	Use:     "run",
	Short:   "Run a simple server instead of reading a full config file",
	Example: "junction run --routing sni --listen 8443 -x socks5://127.0.0.1:7890 --target 443",
	RunE: func(cmd *cobra.Command, _ []string) error {
		entry := new(config.EntryPoint)
		var err error
		var listen string
		if listen, err = cmd.Flags().GetString("listen"); err != nil {
			return err
		}
		if entry.Listen, err = netip.ParseAddrPort(listen); err != nil {
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
			entry.Proxy = append(entry.Proxy, pu)
		}
		var routingValue string
		if routingValue, err = cmd.Flags().GetString("routing"); err != nil {
			return err
		}
		if err = entry.Routing.Set(routingValue); err != nil {
			return err
		}
		if entry.Target, err = cmd.Flags().GetString("target"); err != nil {
			return err
		}

		if entry.Params, err = readParamsFlag(cmd); err != nil {
			return err
		}

		if entry.Timeout, err = cmd.Flags().GetDuration("timeout"); err != nil {
			return err
		}

		if entry.Routing != config.RouterSSHServer && entry.Target == "" {
			return errors.New("target is required for this routing type")
		}

		var cfg config.Config
		cfg.EntryPoints = []config.EntryPoint{
			*entry,
		}
		if dump {
			return dumpConf(&cfg)
		}
		ctx, cancel, err := buildAppContext()
		if err != nil {
			return err
		}
		defer cancel()
		if err = server.Serve(ctx, cfg); err != nil {
			return err
		}
		return nil
	},
}

// init registers the 'run' command and its flags with the root command, marking required flags as necessary.
func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringP("listen", "l", "127.0.0.1:8080", "Address to listen on (e.g., 0.0.0.0:8080,")
	runCmd.Flags().StringSliceP("proxy", "x", nil, "Proxy URLs (multiple or none allowed, e.g., socks5://127.0.0.1:7890)")
	runCmd.Flags().StringP("routing", "r", "", "Routing method (e.g., sni, http-header, tcp-raw)")
	runCmd.Flags().StringP("target", "t", "", "Target (based on routing method)")
	runCmd.Flags().StringSliceP("params", "p", nil, "Extra params as key=value pairs (repeatable)")
	runCmd.Flags().DurationP("timeout", "T", constants.Day, "Timeout for requests")

	requireOrPanic("routing")
}

func requireOrPanic(name string) {
	if err := runCmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}

func readParamsFlag(cmd *cobra.Command) (map[string]string, error) {
	raw, err := cmd.Flags().GetStringSlice("params")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	params := make(map[string]string, len(raw))
	for _, item := range raw {
		key, val, ok := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, errors.New("params must be key=value pairs")
		}
		params[key] = strings.TrimSpace(val)
	}
	return params, nil
}

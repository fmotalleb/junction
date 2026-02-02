/*
Copyright © 2025 Motalleb Fallahnezhad (fmotalleb@gmail.com)

This program is free software; you can redistribute it and/or
modify it under the terms of the GNU General Public License
as published by the Free Software Foundation; either version 2
of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

package cmd

import (
	"context"
	"os"
	"time"

	"github.com/fmotalleb/go-tools/git"
	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/go-tools/reloader"
	"github.com/spf13/cobra"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/server"
)

var (
	dump  = false
	debug = false
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "junction",
	Short: "Lightweight general proxy server that transfers network packets over a proxy or a proxy chain",
	Long: `Junction is a lightweight proxy server for efficient HTTP and HTTPS routing with support for:
  • SNI passthrough for TLS routing without terminating SSL
  • SOCKS5 and SSH proxy + proxy chaining as transfer layer
  • Flexible routing via: SNI (TLS), HTTP
  • Docker-ready deployment
	• Internal support for singbox`,
	Version: git.String(),
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		if debug {
			log.SetDebugDefaults()
		}
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configFile string
		var err error
		var cfg config.Config
		if configFile, err = cmd.Flags().GetString("config"); err != nil {
			return err
		}

		ctx, cancel, err := buildAppContext()
		if err != nil {
			return err
		}
		defer cancel()
		err = reloader.WithOsSignal(
			ctx,
			func(ctx context.Context) error {
				if pErr := config.Parse(ctx, &cfg, configFile); pErr != nil {
					return pErr
				}
				if dump {
					return dumpConf(&cfg)
				}
				if sErr := server.Serve(ctx, cfg); sErr != nil {
					return sErr
				}
				return nil
			},
			time.Minute,
		)
		return err
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// init initializes command-line flags for the root command, including configuration file path, format, debug mode, and dry-run options.
func init() {
	rootCmd.Flags().StringP("config", "c", "", "config file (default: reading config from stdin)")
	rootCmd.Flags().StringP("format", "f", "", "config format (yaml, json, toml, ini, hcl)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")
	rootCmd.PersistentFlags().BoolVar(&dump, "dry-run", false, "just output the config, do not start service")
}

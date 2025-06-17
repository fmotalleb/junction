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
	"os"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/server"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "junction",
	Short: "Lightweight general proxy server that transfers network packets over a proxy or a proxy chain",
	Long: `Junction is a lightweight proxy server for efficient HTTP and HTTPS routing with support for:
  • SNI passthrough for TLS routing without terminating SSL
  • SOCKS5 and SSH proxy + proxy chaining as transfer layer
  • Flexible routing via: SNI (TLS), HTTP
  • Docker-ready deploy with supervisord + sing-box`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgFile, err := cmd.Flags().GetString("config")
		if err != nil {
			return err
		}
		var cfg config.Config
		if err := config.Parse(&cfg, cfgFile); err != nil {
			return err
		}
		if err := server.Serve(cfg); err != nil {
			return err
		}
		select {}
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

func init() {
	rootCmd.Flags().StringP("config", "c", "./junction.toml", "config file")
}

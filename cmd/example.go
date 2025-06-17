/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed example/config.toml
var configData []byte

// exampleCmd represents the example command
var exampleCmd = &cobra.Command{
	Use:   "example",
	Short: "An example config file",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(string(configData))
	},
}

func init() {
	rootCmd.AddCommand(exampleCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// exampleCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// exampleCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

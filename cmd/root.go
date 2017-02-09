package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:  "abcweb",
	Long: `ABCWeb is a tool to help you scaffold and develop Go web applications.`,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		// Print stacktrace if debug enabled
		if viper.GetBool("debug") {
			fmt.Printf("%+v\n", err)
		}
		os.Exit(1)
	}
}

func checkDep(name string) {
	_, err := exec.LookPath(name)
	if err != nil {
		fmt.Printf("Error: could not find %q dependency in $PATH. Please run \"abcweb deps\" to install all missing dependencies.", name)
		os.Exit(1)
	}
}

func init() {
	RootCmd.Flags().BoolP("version", "", false, "Print the ABCWeb version")
	viper.BindPFlags(RootCmd.Flags())
}

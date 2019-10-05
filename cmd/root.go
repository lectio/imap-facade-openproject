package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "imap-facade-openproject",
	Short: "IMAP facade to OpenProjects.",
	Long:  `This is an IMAP facade to OpenProjects.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is conf/imap.toml)")
}

// loadConfig reads in config file.
func loadConfig(cfgFile string, cfgName string, merge bool) {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config "conf/imap.*"
		viper.AddConfigPath("conf")
		viper.SetConfigName(cfgName)
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	var err error
	if merge {
		err = viper.MergeInConfig()
	} else {
		err = viper.ReadInConfig()
	}

	if err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		fmt.Println(err)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		loadConfig(cfgFile, "", false)
	} else {
		loadConfig("", "imap", false)
	}
}

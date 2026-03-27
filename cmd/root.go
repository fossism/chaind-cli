package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "chaind",
	Short: "The Sovereign Data Layer for Personal AI Agents",
	Long: `chaind is a local-first daemon that unifies your WhatsApp, Telegram, 
and Matrix accounts into a secure local SQLite store, exposing a Unix Socket 
IPC for your local AI agents to build context and take actions across platforms.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/chaind/config.toml)")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		configDir := fmt.Sprintf("%s/.config/chaind", home)
		viper.AddConfigPath(configDir)
		viper.SetConfigType("toml")
		viper.SetConfigName("config")
		
		// Ensure config dir exists
		os.MkdirAll(configDir, 0700)
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		// Used config file
	}
}

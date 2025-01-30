package cmd

import (
	configCmd "dgram/cmd/config"
	"dgram/cmd/transcribe"
	"dgram/lib/config"
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

const appName = "dgram"

var cfg *config.Config

func init() {
	cfg = config.NewConfig(appName)
	rootCmd.AddCommand(configCmd.GetCmd(cfg))
	rootCmd.AddCommand(transcribe.GetCmd(cfg))

}

var rootCmd = &cobra.Command{
	Use:   appName,
	Short: "Get the transcription of video and audio files using Deepgram.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cfg.Read()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

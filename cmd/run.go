package cmd

import (
	"log"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/lectio/imap-facade-openproject/facade"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run IMAP facade",
	Long:  `Start the IMAP facade`,
	Run: func(cmd *cobra.Command, args []string) {
		cfgTLS := viper.Sub("tls")
		if cfgTLS != nil {
			if err := facade.InitTLS(cfgTLS); err != nil {
				log.Fatal("Failed to initialize TLS support:", err)
			}
		}
		log.Println("Run facade")
		if s, err := facade.NewFacade(); err != nil {
			log.Fatal("Failed connecting to servers:", err)
		} else {
			defer s.Close()

			// run imap server in goroutine
			go s.Run()

			// Listen for shutdown signals
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)

			// Wait for shutodwn signal.
			s := <-c
			log.Println("Got signal:", s)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

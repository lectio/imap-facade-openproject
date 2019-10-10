package cmd

import (
	"log"

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
			facade.InitTLS(cfgTLS)
		}
		log.Println("Run facade")
		if s, err := facade.NewFacade(); err != nil {
			log.Fatal("Failed connecting to servers:", err)
		} else {
			defer s.Close()
			s.Run()
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

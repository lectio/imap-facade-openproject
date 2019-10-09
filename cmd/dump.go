package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// dumpCmd represents the dump command
var dumpCmd = &cobra.Command{
	Use:   "dump [username] [API key]",
	Short: "Dump list of mailboxes and messages",
	Long:  `Testing tool for dumping mailboxes and messages`,
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		tlsEnabled := false
		if cfgTLS := viper.Sub("tls"); cfgTLS != nil {
			tlsEnabled = cfgTLS.GetBool("enabled")
		}

		cfgIMAP := viper.Sub("imap")
		if cfgIMAP == nil {
			log.Fatal("Missing 'imap'")
		}

		dumpAccount(cfgIMAP, tlsEnabled, args)
	},
}

func init() {
	rootCmd.AddCommand(dumpCmd)
}

func dumpAccount(cfg *viper.Viper, tlsEnabled bool, args []string) {
	addr := cfg.GetString("address")

	// If listen address is 0.0.0.0 connect to 127.0.0.1
	addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)

	log.Println("Connecting to server: ", addr)

	var c *client.Client
	var err error
	// Connect to server
	if tlsEnabled {
		c, err = client.DialTLS(addr, nil)
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	// Don't forget to logout
	defer c.Logout()

	// Login
	if err := c.Login(args[0], args[1]); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")

	// List mailboxes
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	for m := range mailboxes {
		log.Printf("----- Mailbox: %s", m.Name)
		dumpMailbox(c, m.Name)
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}

}

func dumpMailbox(c *client.Client, name string) {

	// Select INBOX
	mbox, err := c.Select(name, false)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("---- Flags: %v", mbox.Flags)

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()

	for msg := range messages {
		fmt.Println("* " + msg.Envelope.Subject)
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}
}

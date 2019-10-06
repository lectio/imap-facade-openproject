package facade

import (
	"log"

	"github.com/spf13/viper"

	"github.com/Neopallium/go-imap/server"

	id "github.com/ProtonMail/go-imap-id"
	idle "github.com/emersion/go-imap-idle"
	move "github.com/emersion/go-imap-move"
	specialuse "github.com/emersion/go-imap-specialuse"
	unselect "github.com/emersion/go-imap-unselect"

	"github.com/lectio/imap-facade-openproject/facade/backend"
)

type ImapFacade struct {
	server *server.Server
}

func NewFacade(cfgIMAP *viper.Viper, cfgOP *viper.Viper) (*ImapFacade, error) {
	// Create a OpenProject backend
	be := backend.New(cfgOP)

	addr := cfgIMAP.GetString("address")

	serverID := id.ID{
		"name": "OpenProject Facade",
	}

	// Create a new server
	s := server.New(be)

	// Add extensions
	s.Enable(idle.NewExtension())
	s.Enable(id.NewExtension(serverID))
	s.Enable(move.NewExtension())
	s.Enable(specialuse.NewExtension())
	s.Enable(unselect.NewExtension())

	s.Addr = addr
	// Since we will use this server for testing only, we can allow plain text
	// authentication over unencrypted connections
	s.AllowInsecureAuth = true

	return &ImapFacade{
		server: s,
	}, nil
}

func (g *ImapFacade) Close() {
}

func (g *ImapFacade) Run() {
	log.Println("Starting IMAP server at:", g.server.Addr)
	if err := g.server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

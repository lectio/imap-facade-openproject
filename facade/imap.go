package facade

import (
	"log"

	"github.com/spf13/viper"

	"github.com/emersion/go-imap/server"

	id "github.com/ProtonMail/go-imap-id"
	idle "github.com/emersion/go-imap-idle"
	move "github.com/emersion/go-imap-move"
	specialuse "github.com/emersion/go-imap-specialuse"
	unselect "github.com/emersion/go-imap-unselect"

	"github.com/lectio/imap-facade-openproject/facade/backend"
)

type ImapFacade struct {
	backend *backend.Backend
	server  *server.Server
}

func NewFacade() (*ImapFacade, error) {
	cfgOP := viper.Sub("openprojects")
	if cfgOP == nil {
		log.Fatal("Missing 'openprojects'")
	}
	cfgIMAP := viper.Sub("imap")
	if cfgIMAP == nil {
		log.Fatal("Missing 'imap'")
	}
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
	if tlsEnabled {
		s.TLSConfig = tlsConfig
	}
	// Since we will use this server for testing only, we can allow plain text
	// authentication over unencrypted connections
	s.AllowInsecureAuth = true

	return &ImapFacade{
		backend: be,
		server:  s,
	}, nil
}

func (g *ImapFacade) Close() {
	g.server.Close()
	g.backend.Close()
}

func (g *ImapFacade) Run() {
	log.Println("Starting IMAP server at:", g.server.Addr)
	var err error
	if tlsEnabled {
		err = g.server.ListenAndServeTLS()
	} else {
		err = g.server.ListenAndServe()
	}
	if err != nil {
		log.Fatal(err)
	}
}

// OpenProject IMAP backend
package backend

import (
	"errors"
	"log"
	"sync"

	"github.com/spf13/viper"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"

	"github.com/lectio/imap-facade-openproject/hal"
)

type Backend struct {
	sync.RWMutex

	// OpenProject
	base string

	users map[string]*User

	updates chan backend.Update
}

func (be *Backend) Login(_ *imap.ConnInfo, username, password string) (backend.User, error) {
	be.Lock()
	defer be.Unlock()

	user, ok := be.users[username]
	if ok {
		// user already exists check password
		if user.password == password {
			log.Printf("--- Login ok: %s", username)
			return user, nil
		}
	}
	// Haven't seen this user before, or password changed.
	if user, err := be.checkUserLogin(username, password); err == nil {
		log.Printf("--- Login ok: %s", username)
		return user, nil
	} else {
		log.Printf("--- Login failed: %v", err)
		return nil, err
	}

	return nil, errors.New("Bad username or password")
}

func (be *Backend) checkUserLogin(username, password string) (*User, error) {
	c := hal.NewHalClient(be.base)
	c.SetAPIKey(password)

	if res, err := c.Get("/api/v3/my_preferences"); err != nil {
		return nil, err
	} else {
		if resErr := res.IsError(); resErr != nil {
			return nil, errors.New(resErr.Message)
		}
	}
	// TODO: get user login from '/api/v3/users/{id}'

	user := NewUser(be, c, username, password)
	be.users[username] = user
	return user, nil
}

func (be *Backend) Updates() <-chan backend.Update {
	return be.updates
}

func (be *Backend) PushUpdate(update backend.Update) {
	wait := update.Done()
	be.updates <- update
	<-wait
}

func New(cfg *viper.Viper) *Backend {
	base := cfg.GetString("base")

	log.Println("OpenProject Backend: ", base)

	return &Backend{
		base:    base,
		users:   make(map[string]*User),
		updates: make(chan backend.Update),
	}
}

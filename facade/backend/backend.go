// OpenProject IMAP backend
package backend

import (
	"errors"
	"log"
	"sync"

	"github.com/spf13/viper"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"

	hal "github.com/lectio/go-json-hal"
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
	}

	return nil, errors.New("Bad username or password")
}

func (be *Backend) checkUserLogin(username, password string) (*User, error) {
	c := hal.NewHalClient(be.base)
	c.SetAPIKey(password)

	res, err := c.Get("/api/v3/my_preferences")
	if err != nil {
		return nil, err
	}

	prefs, ok := res.(*hal.UserPreferences)
	if !ok {
		return nil, errors.New("Failed to load user preferences.")
	}
	userRes, err := prefs.GetUser(c)
	if err != nil {
		return nil, errors.New("Failed to load user details.")
	}
	// Got user details.  Check username.
	if username != userRes.Login() {
		return nil, errors.New("IMAP Username doesn't match OpenProject login.")
	}

	user := NewUser(be, c, userRes, password)
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

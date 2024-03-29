// OpenProject IMAP backend
package backend

import (
	"errors"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/viper"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"

	hal "github.com/lectio/go-json-hal"
)

var (
	emailDomain      = "example.com"
	emailPlaceHolder = "user-{id}@example.com"
	activityName     = "Other"
	wordsPerMinute   = 200.0
)

type Backend struct {
	sync.RWMutex

	// OpenProject
	base           string
	updateInterval int

	// Email template
	emailTemplate *EmailTemplate

	users map[string]*User

	updates chan backend.Update

	cache *Cache
}

func formatEmailAddress(u *hal.User) string {
	if u == nil {
		return ""
	}
	email := u.Email()
	if email == "" {
		email = strings.Replace(emailPlaceHolder, `{id}`, strconv.Itoa(u.Id()), -1)
	}
	return u.Name() + " <" + email + ">"
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

func (be *Backend) GenerateMessage(u *User, w *hal.WorkPackage) (*Message, error) {
	return be.emailTemplate.Generate(u, w)
}

func (be *Backend) LoadAttachment(hc *hal.HalClient, at *hal.Attachment) (io.Reader, error) {
	return be.cache.LoadAttachment(hc, at)
}

func (be *Backend) LoadCachedAddress(hc *hal.HalClient, link *hal.Link) (string, error) {
	return be.cache.LoadCachedAddress(hc, link)
}

func (be *Backend) FindTimeEntryActivityURL(hc *hal.HalClient, name string) (*hal.Link, error) {
	return be.cache.FindTimeEntryActivityURL(hc, name)
}

func (be *Backend) Close() {
	be.cache.Close()
}

func New(cfg *viper.Viper) *Backend {
	base := cfg.GetString("base")
	emailDomain = cfg.GetString("emailDomain")
	emailPlaceHolder = cfg.GetString("emailPlaceHolder")

	activityName = cfg.GetString("timeEntryActivity")
	if activityName == "" {
		// Default to "Other"
		activityName = "Other"
	}
	wordsPerMinute = cfg.GetFloat64("wordsPerMinute")
	if wordsPerMinute == 0 {
		wordsPerMinute = 200.0
	}

	tpl, err := NewEmailTemplate(cfg)
	if err != nil {
		log.Panicf("Failed to load email templates: %v", err)
	}

	cache := NewCache(cfg.Sub("cache"))

	log.Println("OpenProject Backend: ", base)

	return &Backend{
		base:           base,
		updateInterval: cfg.GetInt("updateInterval"),
		users:          make(map[string]*User),
		updates:        make(chan backend.Update),
		emailTemplate:  tpl,
		cache:          cache,
	}
}

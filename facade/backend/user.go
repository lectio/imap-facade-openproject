package backend

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/asdine/storm"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"

	hal "github.com/lectio/go-json-hal"
)

type User struct {
	sync.RWMutex

	hal *hal.HalClient

	backend   *Backend
	username  string
	password  string // Cached password for faster sync
	email     string
	mailboxes map[string]*Mailbox

	// per-user cache
	store storm.Node

	// project to mailbox map
	projMap map[int]*Mailbox

	user *hal.User
}

func NewUser(backend *Backend, hal *hal.HalClient, userRes *hal.User, password string) *User {
	email := userRes.Email()
	if email == "" {
		email = userRes.Login()
	}
	username := userRes.Login()

	// Initialize user storage
	store := backend.cache.GetNode("Users").From(username)
	if err := store.Init(&Mailbox{}); err != nil {
		log.Println("Failed to initialize mailboxes store:", err)
	}

	user := &User{
		backend:   backend,
		hal:       hal,
		user:      userRes,
		username:  username,
		password:  password,
		email:     email,
		mailboxes: map[string]*Mailbox{},
		store:     store,
	}

	// load mailboxes
	var mboxes []*Mailbox
	if err := store.All(&mboxes); err != nil {
		log.Println("Failed to load user's mailboxes:", err)
	}
	for _, mbox := range mboxes {
		user.appendMailbox(mbox, false)
	}

	inbox, _ := user.createMailbox("INBOX", "")
	user.createWelcomeMessage(inbox)

	// Initial update
	user.runUpdate(true)

	// Start background updater
	go user.updater(backend.updateInterval)

	return user
}

func (u *User) createWelcomeMessage(mbox *Mailbox) {
	// Message for tests
	body := "Hi " + u.user.Name() + ",\r\n" +
		"Welcome to the lectio IMAP facade for OpenProjects."
	html := "<html><head></head><body>" + body + "</body></html>"

	msg, _ := buildSimpleMessage("contact@"+emailDomain,
		formatEmailAddress(u.user), "",
		"Welcome new lectio user", body, html)

	mbox.appendMessage(msg)
}

func (u *User) LoadAttachment(hc *hal.HalClient, at *hal.Attachment) (io.Reader, error) {
	return u.backend.LoadAttachment(hc, at)
}

func (u *User) getCachedAddress(link *hal.Link) (string, bool) {
	addr, err := u.backend.LoadCachedAddress(u.hal, link)
	if err != nil {
		log.Printf("Failed to get user details: link=%+v, err=%s", link, err)
		return "", false
	}
	return addr, true
}

func (u *User) updater(interval int) {
	log.Println("User updater: started.")

	// update mailboxes right away.
	u.updateMailboxes()

	for {
		time.Sleep(time.Second * time.Duration(interval))

		// Update project mailboxes
		u.runUpdate(false)
	}
}

func (u *User) runUpdate(firstTime bool) {
	log.Println("Run update.")

	// Update projects
	if err := u.updateProjects(); err != nil {
		log.Printf("Failed to get projects: %v", err)
	}

	if firstTime {
		// skip mailbox update the first time.
		return
	}

	// Update mailboxes
	u.updateMailboxes()
}

func (u *User) updateMailboxes() {
	u.RLock()
	defer u.RUnlock()

	for _, mbox := range u.mailboxes {
		mbox.runUpdate(u.hal)
	}
}

func (u *User) updateProjects() error {
	u.Lock()
	defer u.Unlock()

	// Get first page of projects
	col, err := u.hal.GetCollection("/api/v3/projects")
	if err != nil {
		return fmt.Errorf("Failed to get projects: %s", err)
	}

	return u.createProjects(col)
}

func (u *User) createProjects(col *hal.Collection) error {
	for _, itemRes := range col.Items() {
		proj, ok := itemRes.(*hal.Project)
		if !ok {
			return fmt.Errorf("Invalid resource type: %s", itemRes.ResourceType())
		}
		// Create mailbox if it doesn't exist.
		mbox, _ := u.createProjectMailbox(proj)
		if mbox.project == nil {
			mbox.project = proj
			mbox.ProjectID = proj.Id()
		}
	}

	// Check for next page of projects.
	if col.IsPaginated() {
		if nextCol, err := col.NextPage(u.hal); err == nil {
			return u.createProjects(nextCol)
		}
	}
	return nil
}

func (u *User) Backend() *Backend {
	return u.backend
}

func (u *User) Username() string {
	return u.username
}

func (u *User) ListMailboxes(subscribed bool) (mailboxes []backend.Mailbox, err error) {
	u.RLock()
	defer u.RUnlock()

	for _, mailbox := range u.mailboxes {
		if subscribed && !mailbox.Subscribed {
			continue
		}

		mailboxes = append(mailboxes, mailbox)
	}
	return
}

func (u *User) GetMailbox(name string) (mailbox backend.Mailbox, err error) {
	u.RLock()
	defer u.RUnlock()

	mailbox, ok := u.mailboxes[name]
	if !ok {
		err = errors.New("No such mailbox")
	}
	return
}

func (u *User) updateMailbox(mbox *Mailbox) {
	//log.Printf("--- Update Mailbox: %+v", mbox)
	// Update mailbox
	if err := u.store.Update(mbox); err != nil {
		log.Println("Error updating mailbox:", err)
	}
}

func (u *User) appendMailbox(mbox *Mailbox, isNew bool) {
	mbox.user = u
	name := mbox.Name()

	if isNew {
		// Save mailbox if it is new
		if err := u.store.Save(mbox); err != nil {
			log.Println("Error saving mailbox:", err)
		}
	}
	//log.Printf("--- Append mailbox(%s): id=%d, isNew=%v", mbox.Name(), mbox.Id, isNew)

	// Get mailbox storage
	store := u.store.From("mailboxes").From(strconv.Itoa(mbox.Id))
	mbox.store = store

	mbox.init()
	u.mailboxes[name] = mbox
}

func (u *User) createMailbox(name string, specialUse string) (*Mailbox, error) {
	if mbox, ok := u.mailboxes[name]; ok {
		return mbox, errors.New("Mailbox already exists")
	}

	mbox := NewMailbox(u, name, specialUse)
	u.mailboxes[name] = mbox
	return mbox, nil
}

func (u *User) createProjectMailbox(proj *hal.Project) (*Mailbox, bool) {
	name := proj.Name()
	if mbox, ok := u.mailboxes[name]; ok {
		return mbox, true
	}

	mbox := NewProjectMailbox(u, proj)
	u.mailboxes[name] = mbox

	// Auto subscribe to project mailboxes
	mbox.SetSubscribed(true)

	return mbox, false
}

func (u *User) CreateMailbox(name string) error {
	u.Lock()
	defer u.Unlock()

	_, err := u.createMailbox(name, "")
	return err
}

func (u *User) DeleteMailbox(name string) error {
	u.Lock()
	defer u.Unlock()

	if name == "INBOX" {
		return errors.New("Cannot delete INBOX")
	}
	if _, ok := u.mailboxes[name]; !ok {
		return errors.New("No such mailbox")
	}

	delete(u.mailboxes, name)
	return nil
}

func (u *User) RenameMailbox(existingName, newName string) error {
	u.Lock()
	defer u.Unlock()

	mbox, ok := u.mailboxes[existingName]
	if !ok {
		return errors.New("No such mailbox")
	}

	// Move mailbox to new name.
	mbox.MailboxName = newName
	u.mailboxes[newName] = mbox
	delete(u.mailboxes, existingName)

	if existingName == "INBOX" {
		// Create a new INBOX
		u.createMailbox("INBOX", "")
	}

	return nil
}

func (u *User) PushMailboxUpdate(mbox *Mailbox) {
	update := &backend.MailboxUpdate{}
	update.Update = backend.NewUpdate(u.username, mbox.Name())
	status, err := mbox.status([]imap.StatusItem{imap.StatusMessages, imap.StatusUnseen}, true)
	if err == nil {
		update.MailboxStatus = status
		u.backend.PushUpdate(update)
	} else {
		// Failed to get current mailbox status.
		log.Printf("Failed push mailbox update: %s", err)
	}
}

func (u *User) PushMessageUpdate(mailbox string, msg *imap.Message) {
	update := &backend.MessageUpdate{}
	update.Update = backend.NewUpdate(u.username, mailbox)
	update.Message = msg
	u.backend.PushUpdate(update)
}

func (u *User) PushExpungeUpdate(mailbox string, seqNum uint32) {
	update := &backend.ExpungeUpdate{}
	update.Update = backend.NewUpdate(u.username, mailbox)
	update.SeqNum = seqNum
	u.backend.PushUpdate(update)
}

func (u *User) Logout() error {
	return nil
}

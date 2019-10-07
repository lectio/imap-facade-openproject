package backend

import (
	"errors"
	"fmt"
	"log"
	"sync"

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

	user *hal.User
}

func NewUser(backend *Backend, hal *hal.HalClient, userRes *hal.User, password string) *User {
	email := userRes.Email()
	if email == "" {
		email = userRes.Login()
	}

	user := &User{
		backend:   backend,
		hal:       hal,
		user:      userRes,
		username:  userRes.Login(),
		password:  password,
		email:     email,
		mailboxes: map[string]*Mailbox{},
	}

	// Message for tests
	body := "Hi " + userRes.Name() + ",\r\n" +
		"Welcome to the lectio IMAP facade for OpenProjects."
	html := "<html><head></head><body>" + body + "</body></html>"

	msg, _ := buildSimpleMessage("contact@example.org",
		userRes.Name()+" <"+email+">",
		"Welcome new lectio user", body, html)

	user.createMailbox("INBOX", "")
	inbox := user.mailboxes["INBOX"]
	inbox.appendMessage(msg)

	return user
}

func (u *User) updateProjects() error {
	// Get list of projects
	col, err := u.hal.GetCollection("/api/v3/projects")
	if err != nil {
		return fmt.Errorf("Failed to get projects: %s", err)
	}

	for _, itemRes := range col.Items() {
		proj, ok := itemRes.(*hal.Project)
		if !ok {
			return fmt.Errorf("Invalid resource type: %s", itemRes.ResourceType())
		}
		name := proj.Name()
		// Check if mailbox already exists
		if _, ok := u.mailboxes[name]; ok {
			continue
		}
		// Create IMAP mailbox for Project
		log.Printf("Create Project folder: %v", name)
		mbox := NewProjectMailbox(u, proj)
		u.mailboxes[name] = mbox
		// Auto subscribe to project mailboxes
		mbox.SetSubscribed(true)
	}

	return nil
}

func (u *User) Username() string {
	return u.username
}

func (u *User) ListMailboxes(subscribed bool) (mailboxes []backend.Mailbox, err error) {
	u.RLock()
	defer u.RUnlock()

	// Update project mailboxes
	if err := u.updateProjects(); err != nil {
		log.Printf("Failed to get projects: %v", err)
	}

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

func (u *User) createMailbox(name string, specialUse string) (mailbox *Mailbox, err error) {
	if mbox, ok := u.mailboxes[name]; ok {
		return mbox, errors.New("Mailbox already exists")
	}

	mbox := NewMailbox(u, name, specialUse)
	u.mailboxes[name] = mbox
	return mbox, nil
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
	mbox.name = newName
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
	update.Update = backend.NewUpdate(u.username, mbox.name)
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

package backend

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asdine/storm"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"

	specialuse "github.com/emersion/go-imap-specialuse"

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

	// Time Entries
	teLock      sync.RWMutex
	activity    *hal.Link
	timeEntries map[string]*hal.TimeEntry

	user *hal.User
}

func NewUser(backend *Backend, hc *hal.HalClient, userRes *hal.User, password string) *User {
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
		backend:     backend,
		hal:         hc,
		user:        userRes,
		username:    username,
		password:    password,
		email:       email,
		mailboxes:   map[string]*Mailbox{},
		store:       store,
		timeEntries: map[string]*hal.TimeEntry{},
	}

	// Get time entry activity url
	if actLink, err := backend.FindTimeEntryActivityURL(hc, activityName); err == nil {
		user.activity = actLink
	} else {
		log.Fatal("Failed to find time entry activity url:", err)
	}

	// Load time entries
	if err := user.loadTimeEntries(); err != nil {
		log.Println("Error loading user's time entries:", err)
	}

	// load mailboxes
	var mboxes []*Mailbox
	if err := store.All(&mboxes); err != nil {
		log.Println("Failed to load user's mailboxes:", err)
	}
	for _, mbox := range mboxes {
		user.appendMailbox(mbox, false)
	}

	if inbox, err := user.createMailbox("INBOX", ""); err == nil {
		user.createWelcomeMessage(inbox)
	}
	if _, err := user.createMailbox("Trash", specialuse.Trash); err != nil {
		log.Println("Failed to create mailbox:", err)
	}

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

func (u *User) updateWorkPackageFlags(msg *Message) error {
	u.teLock.Lock()
	defer u.teLock.Unlock()

	te, err := u.getTimeEntry(msg.WorkPackageID, true)
	if err != nil {
		return fmt.Errorf("Error getting time entry for work package: %s", err)
	}
	flags := strings.Join(msg.Flags, ",")
	te.SetComment("plain", flags, flags)

	// Check for 'Seen' flag
	seen := false
	for _, flag := range msg.Flags {
		if flag == imap.SeenFlag {
			seen = true
		}
	}

	if seen {
		te.SetHours(1 * time.Minute)
		te.SetSpentOn(time.Now())
	} else {
		te.SetHours(0)
	}

	// Record changes.
	if res, err := te.Update(u.hal); err != nil {
		return fmt.Errorf("Error updating time entry for work package: %s", err)
	} else {
		// Store updated time entry
		if updatedEntry, ok := res.(*hal.TimeEntry); ok {
			te = updatedEntry
		}
	}
	// Get Work package url
	workLink := te.GetLink("workPackage")
	if workLink == nil {
		// Not a work package time entry, ignore it.
		return fmt.Errorf("Updated time entry missing work package url.")
	}
	workURL := workLink.Href

	// Store updated time entry
	u.timeEntries[workURL] = te

	return nil
}

func (u *User) loadWorkPackageFlags(msg *Message) {
	u.teLock.Lock()
	defer u.teLock.Unlock()

	te, _ := u.getTimeEntry(msg.WorkPackageID, false)
	if te == nil {
		// no time entry or error.  Don't modify the Message.
		return
	}
	comment := te.Comment()
	if comment != nil && comment.Raw != "" {
		flags := strings.Split(comment.Raw, ",")
		if len(flags) > 0 {
			msg.Flags = flags
		}
	}
}

func (u *User) getTimeEntry(work_id int, create bool) (*hal.TimeEntry, error) {
	workURL := fmt.Sprintf("/api/v3/work_packages/%d", work_id)

	// Look for existing Time Entry
	if te, ok := u.timeEntries[workURL]; ok {
		return te, nil
	}
	if !create {
		// Don't create time entry.
		return nil, nil
	}

	// Load work package
	var ok bool
	var w *hal.WorkPackage
	if res, err := u.hal.Get(workURL); err != nil {
		return nil, fmt.Errorf("Failed to load work package: %d", work_id)
	} else {
		w, ok = res.(*hal.WorkPackage)
		if !ok {
			return nil, fmt.Errorf("Expected a WorkPackage resource: %+v", res)
		}
	}

	// Test creating TimeEntry
	te := hal.NewTimeEntry()
	te.SetHours(1 * time.Second)
	te.SetComment("plain", "", "")
	te.SetActivity(u.activity.Href)
	if res, err := w.AddTimeEntry(u.hal, te); err != nil {
		return nil, err
	} else {
		te, ok = res.(*hal.TimeEntry)
		if !ok {
			return nil, fmt.Errorf("Expected a TimeEntry resource: %+v", res)
		}
	}

	// Store time entry
	u.timeEntries[workURL] = te

	return te, nil
}

func (u *User) loadTimeEntries() error {

	// Get first page of time entries
	f := hal.NewFilters().Filter("user", "=", u.user.Id())
	col, err := u.hal.GetFilteredCollection("/api/v3/time_entries", f)
	if err != nil {
		return fmt.Errorf("Failed to get time entries: %s", err)
	}

	return u.processTimeEntries(col)
}

func (u *User) processTimeEntries(col *hal.Collection) error {
	for _, itemRes := range col.Items() {
		te, ok := itemRes.(*hal.TimeEntry)
		if !ok {
			return fmt.Errorf("Invalid resource type: %s", itemRes.ResourceType())
		}
		u.processTimeEntry(te)
	}

	// Check for next page of projects.
	if col.IsPaginated() {
		if nextCol, err := col.NextPage(u.hal); err == nil {
			return u.createProjects(nextCol)
		}
	}
	return nil
}

func (u *User) processTimeEntry(te *hal.TimeEntry) {
	// Get Work package url
	workLink := te.GetLink("workPackage")
	if workLink == nil {
		// Not a work package time entry, ignore it.
		return
	}
	workURL := workLink.Href

	updatedAt := te.GetUpdatedAt()
	if updatedAt == nil {
		// Missing 'updatedAt'
		return
	}

	u.teLock.Lock()
	defer u.teLock.Unlock()

	// Check if current time entry is newer
	if cur, ok := u.timeEntries[workURL]; ok {
		curUpdatedAt := cur.GetUpdatedAt()
		if curUpdatedAt.After(*updatedAt) {
			// already have the newest
			return
		}
		// Got a newer time entry
	}

	u.timeEntries[workURL] = te
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
	_ = mbox.SetSubscribed(true)

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
		if _, err := u.createMailbox("INBOX", ""); err != nil {
			log.Println("Error re-creating INBOX:", err)
		}
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

package backend

import (
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/asdine/storm"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/backendutil"

	hal "github.com/lectio/go-json-hal"
)

var Delimiter = "/"

type Mailbox struct {
	sync.RWMutex

	Id          int    `storm:"id,increment"`
	MailboxName string `storm:"unique" json:"Name"`

	Flags       []string
	Attributes  []string
	Subscribed  bool
	UidValidity uint32

	msgs []*Message

	// Mailbox storage
	store storm.Node

	user *User

	// OpenProject Fields
	ProjectID int `json:",omitempty"`

	project *hal.Project
	// Map WorkPackage ID to message
	workMap map[int]*Message
}

func NewMailbox(user *User, name string, specialUse string) *Mailbox {
	mbox := &Mailbox{
		MailboxName: name,
		UidValidity: uint32(time.Now().Nanosecond()),
		Flags: []string{
			imap.AnsweredFlag,
			imap.FlaggedFlag,
			imap.DeletedFlag,
			imap.SeenFlag,
			imap.DraftFlag,
			"nonjunk",
		},
	}
	if specialUse != "" {
		mbox.Attributes = []string{specialUse}
	}
	user.appendMailbox(mbox, true)
	return mbox
}

func NewProjectMailbox(user *User, project *hal.Project) *Mailbox {
	mbox := NewMailbox(user, project.Name(), "")
	mbox.project = project
	mbox.ProjectID = project.Id()
	return mbox
}

func (mbox *Mailbox) init() {
	mbox.msgs = []*Message{}
	mbox.workMap = make(map[int]*Message)

	// Initialize mailbox storage
	if err := mbox.store.Init(&Message{}); err != nil {
		log.Println("Failed to initialize mailbox's message store:", err)
	}

	// load messages
	if err := mbox.store.All(&mbox.msgs); err != nil {
		log.Println("Failed to load mailbox's messages:", err)
	}
	for _, msg := range mbox.msgs {
		msg.mbox = mbox
		id := msg.WorkPackageID
		if id > 0 {
			mbox.workMap[id] = msg
		}
	}

}

func (mbox *Mailbox) checkWorkPackage(w *hal.WorkPackage) bool {
	mbox.RLock()
	defer mbox.RUnlock()

	id := w.Id()
	// Check for existing message.
	if _, ok := mbox.workMap[id]; ok {
		// TODO: check if work package has changed.
		return false
	}
	return true
}

func (mbox *Mailbox) workPackageToMessage(c *hal.HalClient, w *hal.WorkPackage) error {
	id := w.Id()
	// Check if work package needs to be created/updated
	if !mbox.checkWorkPackage(w) {
		return nil
	}

	fmt.Printf("-- Create message for Work Package: %s\n", w.Subject())

	msg, err := mbox.user.GenerateMessage(w)
	if err != nil {
		return err
	}

	// Modify mailbox.  Append new message.
	mbox.Lock()
	defer mbox.Unlock()
	// Add message to mailbox
	mbox.appendMessage(msg)

	// map work package to message
	mbox.workMap[id] = msg

	return nil
}

func (mbox *Mailbox) createWorkPackages(c *hal.HalClient, col *hal.Collection) error {
	log.Printf("-- Load work packages from page: %d", col.Offset())
	for _, itemRes := range col.Items() {
		work, ok := itemRes.(*hal.WorkPackage)
		if !ok {
			log.Printf("Invalid resource type: %s", itemRes.ResourceType())
			continue
		}
		if err := mbox.workPackageToMessage(c, work); err != nil {
			log.Printf("--- Failed to create message from work package: %s", work.Subject())
		}
	}

	// Check for next page.
	if col.IsPaginated() {
		if nextCol, err := col.NextPage(c); err == nil {
			return mbox.createWorkPackages(c, nextCol)
		}
	}
	return nil
}

func (mbox *Mailbox) runUpdate(c *hal.HalClient) {
	// Update work packages
	if err := mbox.updateWorkPackages(c); err != nil {
		log.Println("Error updating work packages:", err)
	}
}

func (mbox *Mailbox) updateWorkPackages(c *hal.HalClient) error {
	if mbox.project == nil {
		// Not a project folder.
		return nil
	}
	// Get work package
	col, err := mbox.project.GetWorkPackages(c)
	if err != nil {
		return err
	}
	if err := mbox.createWorkPackages(c, col); err != nil {
		return err
	}
	log.Printf("------------- Finished loading work packages.")
	return nil
}

func (mbox *Mailbox) Name() string {
	return mbox.MailboxName
}

func (mbox *Mailbox) Info() (*imap.MailboxInfo, error) {
	mbox.RLock()
	defer mbox.RUnlock()

	info := &imap.MailboxInfo{
		Attributes: mbox.Attributes,
		Delimiter:  Delimiter,
		Name:       mbox.MailboxName,
	}
	return info, nil
}

func (mbox *Mailbox) uidNext() uint32 {
	var uid uint32
	for _, msg := range mbox.msgs {
		if msg.Uid > uid {
			uid = msg.Uid
		}
	}
	uid++
	return uid
}

type messageStats struct {
	unseenSeqNum uint32
	unseenCount  uint32
}

func (mbox *Mailbox) getMsgStats() messageStats {
	stats := messageStats{}
	for i, msg := range mbox.msgs {

		seen := false
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				seen = true
				break
			}
		}

		if !seen {
			stats.unseenCount++
			seqNum := uint32(i + 1)
			if seqNum > stats.unseenSeqNum {
				stats.unseenSeqNum = seqNum
			}
		}
	}
	return stats
}

func (mbox *Mailbox) status(items []imap.StatusItem, flags bool) (*imap.MailboxStatus, error) {
	status := imap.NewMailboxStatus(mbox.MailboxName, items)
	if flags {
		// Copy flags slice (don't re-use slice)
		flags := append(mbox.Flags[:0:0], mbox.Flags...)
		status.Flags = flags
		status.PermanentFlags = append(flags, "\\*")
	}
	msgStats := mbox.getMsgStats()
	status.UnseenSeqNum = msgStats.unseenSeqNum

	for _, name := range items {
		switch name {
		case imap.StatusMessages:
			status.Messages = uint32(len(mbox.msgs))
		case imap.StatusUidNext:
			status.UidNext = mbox.uidNext()
		case imap.StatusUidValidity:
			status.UidValidity = mbox.UidValidity
		case imap.StatusRecent:
			status.Recent = 0 // TODO
		case imap.StatusUnseen:
			status.Unseen = msgStats.unseenCount
		}
	}

	return status, nil
}

func (mbox *Mailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	mbox.RLock()
	defer mbox.RUnlock()

	return mbox.status(items, true)
}

func (mbox *Mailbox) SetSubscribed(subscribed bool) error {
	mbox.Lock()
	defer mbox.Unlock()

	mbox.Subscribed = subscribed

	mbox.saveMailbox()
	return nil
}

func (mbox *Mailbox) Check() error {
	return nil
}

func (mbox *Mailbox) ListMessages(uid bool, seqSet *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	mbox.RLock()
	defer mbox.RUnlock()
	defer close(ch)

	for i, msg := range mbox.msgs {
		seqNum := uint32(i + 1)

		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = seqNum
		}
		if !seqSet.Contains(id) {
			continue
		}

		m, err := msg.Fetch(seqNum, items)
		if err != nil {
			continue
		}

		ch <- m
	}

	return nil
}

func (mbox *Mailbox) SearchMessages(uid bool, criteria *imap.SearchCriteria) ([]uint32, error) {
	mbox.RLock()
	defer mbox.RUnlock()

	var ids []uint32
	for i, msg := range mbox.msgs {
		seqNum := uint32(i + 1)

		ok, err := msg.Match(seqNum, criteria)
		if err != nil || !ok {
			continue
		}

		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = seqNum
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (mbox *Mailbox) saveMailbox() {
	mbox.user.updateMailbox(mbox)
}

func (mbox *Mailbox) getMessageBody(msg *Message) []byte {
	if buf, err := mbox.store.GetBytes("bodies", msg.Uid); err == nil {
		return buf
	} else {
		log.Println("Failed to load message body:", err)
	}
	return nil
}

func (mbox *Mailbox) deleteMessage(msg *Message) {
	// Delete message body.
	if err := mbox.store.Delete("bodies", msg.Uid); err != nil {
		log.Println("Failed to delete message body:", err)
	}

	// Delete message
	if err := mbox.store.DeleteStruct(msg); err != nil {
		log.Println("Error deleting message in mailbox:", err)
	}
}

func (mbox *Mailbox) appendMessage(msg *Message) {
	msg.mbox = mbox

	// Save message
	if err := mbox.store.Save(msg); err != nil {
		log.Println("Error saving message in mailbox:", err)
	}

	// Save message body.
	if err := mbox.store.SetBytes("bodies", msg.Uid, msg.body); err != nil {
		log.Println("Failed to store message body:", err)
	}
	// Don't keep message body in memory
	msg.body = nil

	mbox.msgs = append(mbox.msgs, msg)
}

func (mbox *Mailbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	mbox.Lock()
	defer mbox.Unlock()

	if date.IsZero() {
		date = time.Now()
	}

	b, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}

	mbox.appendMessage(&Message{
		Date:  date,
		Size:  uint32(len(b)),
		Flags: append(flags, imap.RecentFlag),
		body:  b,
	})
	mbox.Flags = backendutil.UpdateFlags(mbox.Flags, imap.AddFlags, flags)
	mbox.saveMailbox()
	mbox.user.PushMailboxUpdate(mbox)
	return nil
}

func (mbox *Mailbox) pushMessageUpdate(uid bool, msg *Message, seqNum uint32) {
	// Update message
	if err := mbox.store.Update(msg); err != nil {
		log.Println("Error updating message in mailbox:", err)
	}

	// If message is for a work package, then update flags in OpenProject
	if msg.WorkPackageID > 0 {
		if err := mbox.user.updateWorkPackageFlags(msg); err != nil {
			log.Println("Error updating work package flags:", err)
		}
	}

	items := []imap.FetchItem{imap.FetchFlags}
	if uid {
		items = append(items, imap.FetchUid)
	}
	uMsg := imap.NewMessage(seqNum, items)
	uMsg.Flags = msg.Flags
	uMsg.Uid = msg.Uid
	mbox.user.PushMessageUpdate(mbox.MailboxName, uMsg)
}

func CompareFlags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func UpdateFlags(current []string, op imap.FlagsOp, flags []string) ([]string, bool) {
	origFlags := append(current[:0:0], current...)
	current = backendutil.UpdateFlags(current, op, flags)
	changed := !CompareFlags(current, origFlags)
	return current, changed
}

func (mbox *Mailbox) UpdateMessagesFlags(uid bool, seqset *imap.SeqSet, op imap.FlagsOp, flags []string) error {
	mbox.Lock()
	defer mbox.Unlock()

	for i, msg := range mbox.msgs {
		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = uint32(i + 1)
		}
		if !seqset.Contains(id) {
			continue
		}

		if newFlags, changed := UpdateFlags(msg.Flags, op, flags); changed {
			msg.Flags = newFlags
			mbox.pushMessageUpdate(uid, msg, uint32(i+1))
		}
	}

	// Update mailbox flags list
	if op == imap.AddFlags || op == imap.SetFlags {
		if newFlags, changed := UpdateFlags(mbox.Flags, imap.AddFlags, flags); changed {
			mbox.Flags = newFlags
			mbox.saveMailbox()
			mbox.user.PushMailboxUpdate(mbox)
		}
	}

	return nil
}

// TODO: CopyMessages must also lock destination mailbox.
func (mbox *Mailbox) CopyMessages(uid bool, seqset *imap.SeqSet, destName string) error {
	mbox.Lock()
	defer mbox.Unlock()

	dest, ok := mbox.user.mailboxes[destName]
	if !ok {
		return backend.ErrNoSuchMailbox
	}

	for i, msg := range mbox.msgs {
		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = uint32(i + 1)
		}
		if !seqset.Contains(id) {
			continue
		}

		msgCopy := msg.copy()
		dest.appendMessage(msgCopy)
	}
	dest.saveMailbox()
	mbox.user.PushMailboxUpdate(dest)

	return nil
}

// TODO: MoveMessages must also lock destination mailbox.
func (mbox *Mailbox) MoveMessages(uid bool, seqset *imap.SeqSet, destName string) error {
	mbox.Lock()
	defer mbox.Unlock()

	dest, ok := mbox.user.mailboxes[destName]
	if !ok {
		return backend.ErrNoSuchMailbox
	}

	flags := []string{imap.DeletedFlag}
	for i, msg := range mbox.msgs {
		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = uint32(i + 1)
		}
		if !seqset.Contains(id) {
			continue
		}

		msgCopy := msg.copy()
		dest.appendMessage(msgCopy)
		// Mark source message as deleted
		msg.Flags = backendutil.UpdateFlags(msg.Flags, imap.AddFlags, flags)
	}

	dest.saveMailbox()
	mbox.user.PushMailboxUpdate(dest)
	mbox.saveMailbox()
	mbox.user.PushMailboxUpdate(mbox)
	return mbox.expunge()
}

func (mbox *Mailbox) expunge() error {
	for i := len(mbox.msgs) - 1; i >= 0; i-- {
		msg := mbox.msgs[i]

		deleted := false
		for _, flag := range msg.Flags {
			if flag == imap.DeletedFlag {
				deleted = true
				break
			}
		}

		if deleted {
			mbox.deleteMessage(msg)
			mbox.msgs = append(mbox.msgs[:i], mbox.msgs[i+1:]...)
			// send expunge update
			mbox.user.PushExpungeUpdate(mbox.MailboxName, uint32(i+1))
		}
	}

	return nil
}

func (mbox *Mailbox) Expunge() error {
	mbox.Lock()
	defer mbox.Unlock()

	return mbox.expunge()
}

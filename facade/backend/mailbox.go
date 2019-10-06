package backend

import (
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/Neopallium/go-imap"
	"github.com/Neopallium/go-imap/backend"
	"github.com/Neopallium/go-imap/backend/backendutil"

	"github.com/lectio/imap-facade-openproject/hal"
)

var Delimiter = "/"

type Mailbox struct {
	sync.RWMutex

	Flags       []string
	Attributes  []string
	Subscribed  bool
	Messages    []*Message
	UidValidity uint32

	name string
	user *User

	project *hal.Project
	// Map WorkPackage ID to message
	workMap map[int]*Message

	nextUpdate time.Time
}

func NewMailbox(user *User, name string, specialUse string) *Mailbox {
	mbox := &Mailbox{
		name: name, user: user,
		UidValidity: uint32(time.Now().Nanosecond()),
		Messages:    []*Message{},
		Flags: []string{
			imap.AnsweredFlag,
			imap.FlaggedFlag,
			imap.DeletedFlag,
			imap.SeenFlag,
			imap.DraftFlag,
			"nonjunk",
		},
		workMap: make(map[int]*Message),
	}
	if specialUse != "" {
		mbox.Attributes = []string{specialUse}
	}
	return mbox
}

func NewProjectMailbox(user *User, project *hal.Project) *Mailbox {
	mbox := NewMailbox(user, project.Name(), "")
	mbox.project = project
	return mbox
}

func (mbox *Mailbox) workPackageToMessage(w *hal.WorkPackage) error {
	id := w.Id()
	// Check for existing message.
	if _, ok := mbox.workMap[id]; ok {
		// TODO: check if work package has changed.
		return nil
	}

	log.Printf("-- Create message for Work Package: %s", w.Subject())
	u := mbox.user
	// Message for tests
	body := "From: contact@example.org\r\n" +
		"To: " + u.user.Name() + " <" + u.email + ">\r\n" +
		"Date: Wed, 11 May 2016 14:31:59 +0000\r\n" +
		"Message-ID: <0000000@localhost/>\r\n" +
		"Content-Type: text/plain\r\n" +
		"Subject: " + w.Subject() + "\r\n" +
		"\r\n"

	desc := w.Description()
	if desc != nil {
		body += desc.Raw
	}

	msg := &Message{
		Date:  time.Now(),
		Flags: []string{},
		Size:  uint32(len(body)),
		Body:  []byte(body),
	}
	mbox.appendMessage(msg)

	// map work package to message
	mbox.workMap[id] = msg
	return nil
}

func (mbox *Mailbox) createWorkPackages(col *hal.Collection) error {
	log.Printf("-- Load work packages from page: %d", col.Offset())
	for _, itemRes := range col.Items() {
		work, ok := itemRes.(*hal.WorkPackage)
		if !ok {
			return fmt.Errorf("Invalid resource type: %s", itemRes.ResourceType())
		}
		if err := mbox.workPackageToMessage(work); err != nil {
			log.Printf("--- Failed to create message from work package: %s", work.Subject())
		}
	}
	// Check for next page.
	if col.IsPaginated() {
		if nextCol, err := col.NextPage(mbox.user.hal); err == nil {
			return mbox.createWorkPackages(nextCol)
		}
	}
	return nil
}

func (mbox *Mailbox) updateWorkPackages() error {
	if mbox.project == nil {
		// Not a project folder.
		return nil
	}
	// Only check for new work packages every 30 seconds.
	if mbox.nextUpdate.After(time.Now()) {
		// Skip update
		return nil
	}
	// Get work package
	col, err := mbox.project.GetWorkPackages(mbox.user.hal)
	if err != nil {
		return err
	}
	if err := mbox.createWorkPackages(col); err != nil {
		return err
	}
	// Schedule next update 30 seconds.
	mbox.nextUpdate = time.Now().Add(time.Second * 30)
	log.Printf("------------- Finished loading work packages.")
	return nil
}

func (mbox *Mailbox) Name() string {
	return mbox.name
}

func (mbox *Mailbox) Info() (*imap.MailboxInfo, error) {
	mbox.RLock()
	defer mbox.RUnlock()

	info := &imap.MailboxInfo{
		Attributes: mbox.Attributes,
		Delimiter:  Delimiter,
		Name:       mbox.name,
	}
	return info, nil
}

func (mbox *Mailbox) uidNext() uint32 {
	var uid uint32
	for _, msg := range mbox.Messages {
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
	for i, msg := range mbox.Messages {

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
	// Update work packages
	mbox.updateWorkPackages()

	status := imap.NewMailboxStatus(mbox.name, items)
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
			status.Messages = uint32(len(mbox.Messages))
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

	return nil
}

func (mbox *Mailbox) Check() error {
	return nil
}

func (mbox *Mailbox) ListMessages(uid bool, seqSet *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	mbox.RLock()
	defer mbox.RUnlock()
	defer close(ch)

	for i, msg := range mbox.Messages {
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
	for i, msg := range mbox.Messages {
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

func (mbox *Mailbox) appendMessage(msg *Message) {
	msg.Uid = mbox.uidNext()
	mbox.Messages = append(mbox.Messages, msg)
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
		Body:  b,
	})
	mbox.Flags = backendutil.UpdateFlags(mbox.Flags, imap.AddFlags, flags)
	mbox.user.PushMailboxUpdate(mbox)
	return nil
}

func (mbox *Mailbox) pushMessageUpdate(uid bool, msg *Message, seqNum uint32) {
	items := []imap.FetchItem{imap.FetchFlags}
	if uid {
		items = append(items, imap.FetchUid)
	}
	uMsg := imap.NewMessage(seqNum, items)
	uMsg.Flags = msg.Flags
	uMsg.Uid = msg.Uid
	mbox.user.PushMessageUpdate(mbox.name, uMsg)
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

	for i, msg := range mbox.Messages {
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
			mbox.user.PushMailboxUpdate(mbox)
		}
	}

	return nil
}

func (mbox *Mailbox) CopyMessages(uid bool, seqset *imap.SeqSet, destName string) error {
	mbox.Lock()
	defer mbox.Unlock()

	dest, ok := mbox.user.mailboxes[destName]
	if !ok {
		return backend.ErrNoSuchMailbox
	}

	for i, msg := range mbox.Messages {
		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = uint32(i + 1)
		}
		if !seqset.Contains(id) {
			continue
		}

		msgCopy := *msg
		msgCopy.Uid = dest.uidNext()
		dest.Messages = append(dest.Messages, &msgCopy)
	}
	mbox.user.PushMailboxUpdate(dest)

	return nil
}

func (mbox *Mailbox) MoveMessages(uid bool, seqset *imap.SeqSet, destName string) error {
	mbox.Lock()
	defer mbox.Unlock()

	dest, ok := mbox.user.mailboxes[destName]
	if !ok {
		return backend.ErrNoSuchMailbox
	}

	flags := []string{imap.DeletedFlag}
	for i, msg := range mbox.Messages {
		var id uint32
		if uid {
			id = msg.Uid
		} else {
			id = uint32(i + 1)
		}
		if !seqset.Contains(id) {
			continue
		}

		msgCopy := *msg
		msgCopy.Uid = dest.uidNext()
		dest.Messages = append(dest.Messages, &msgCopy)
		// Mark source message as deleted
		msg.Flags = backendutil.UpdateFlags(msg.Flags, imap.AddFlags, flags)
	}

	mbox.user.PushMailboxUpdate(dest)
	mbox.user.PushMailboxUpdate(mbox)
	return mbox.expunge()
}

func (mbox *Mailbox) expunge() error {
	for i := len(mbox.Messages) - 1; i >= 0; i-- {
		msg := mbox.Messages[i]

		deleted := false
		for _, flag := range msg.Flags {
			if flag == imap.DeletedFlag {
				deleted = true
				break
			}
		}

		if deleted {
			mbox.Messages = append(mbox.Messages[:i], mbox.Messages[i+1:]...)
			// send expunge update
			mbox.user.PushExpungeUpdate(mbox.name, uint32(i+1))
		}
	}

	return nil
}

func (mbox *Mailbox) Expunge() error {
	mbox.Lock()
	defer mbox.Unlock()

	return mbox.expunge()
}

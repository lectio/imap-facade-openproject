package backend

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/backendutil"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/textproto"

	"github.com/jordan-wright/email"
)

type Message struct {
	Uid   uint32 `storm:"id,increment"`
	Date  time.Time
	Size  uint32
	Flags []string

	// Work Package message fields
	WorkPackageID int `json:",omitempty"`

	// Don't keep message bodies loaded in memory
	body []byte
	mbox *Mailbox
}

func (m *Message) copy() *Message {
	msgCopy := *m
	msgCopy.body = m.getBody()
	return &msgCopy
}

func (m *Message) getBody() []byte {
	if m.body == nil {
		return m.mbox.getMessageBody(m)
	}
	return m.body
}

func buildSimpleMessage(from, to, cc, subject, text, html string) (*Message, error) {
	e := email.NewEmail()
	e.From = from
	if to != "" {
		e.To = []string{to}
	}
	if cc != "" {
		e.Cc = []string{cc}
	}
	e.Subject = subject
	e.Text = []byte(text)
	e.HTML = []byte(html)

	buf, err := e.Bytes()
	if err != nil {
		return nil, err
	}
	msg := &Message{
		Date:  time.Now(),
		Flags: []string{},
		Size:  uint32(len(buf)),
		body:  buf,
	}

	return msg, nil
}

func (m *Message) entity() (*message.Entity, error) {
	ent, err := message.Read(bytes.NewReader(m.getBody()))
	if err != nil {
		log.Printf("Failed decode message body: %s", err)
	}
	return ent, err
}

func (m *Message) headerAndBody() (textproto.Header, io.Reader, error) {
	body := bufio.NewReader(bytes.NewReader(m.getBody()))
	hdr, err := textproto.ReadHeader(body)
	if err != nil {
		log.Printf("Failed decode message headers and body: %s", err)
	}
	return hdr, body, err
}

func (m *Message) Fetch(seqNum uint32, items []imap.FetchItem) (*imap.Message, error) {
	fetched := imap.NewMessage(seqNum, items)
	for _, item := range items {
		switch item {
		case imap.FetchEnvelope:
			hdr, _, err := m.headerAndBody()
			if err != nil {
				return nil, err
			}
			if envelope, err := backendutil.FetchEnvelope(hdr); err != nil {
				log.Printf("BackendUtil: Failed to fetch message envelopelope: %s", err)
				return nil, err
			} else {
				fetched.Envelope = envelope
			}
		case imap.FetchBody, imap.FetchBodyStructure:
			hdr, body, err := m.headerAndBody()
			if err != nil {
				return nil, err
			}
			if body, err := backendutil.FetchBodyStructure(hdr, body, item == imap.FetchBodyStructure); err != nil {
				log.Printf("BackendUtil: Failed to fetch message body structure: %s", err)
				return nil, err
			} else {
				fetched.BodyStructure = body
			}
		case imap.FetchFlags:
			// Copy flags, don't return reference to message's flags slice.
			flags := append(m.Flags[:0:0], m.Flags...)
			fetched.Flags = flags
		case imap.FetchInternalDate:
			fetched.InternalDate = m.Date
		case imap.FetchRFC822Size:
			fetched.Size = m.Size
		case imap.FetchUid:
			fetched.Uid = m.Uid
		default:
			section, err := imap.ParseBodySectionName(item)
			if err != nil {
				log.Printf("Fetch: Unknown body section name %v: %s", item, err)
				break
			}

			body := bufio.NewReader(bytes.NewReader(m.getBody()))
			hdr, err := textproto.ReadHeader(body)
			if err != nil {
				log.Printf("Fetch: Failed to decode headers & body: %s", err)
				return nil, err
			}

			l, err := backendutil.FetchBodySection(hdr, body, section)
			if err != nil {
				log.Printf("Fetch: failed to fetch body section: %s", err)
			}
			fetched.Body[section] = l
		}
	}

	return fetched, nil
}

func (m *Message) Match(seqNum uint32, c *imap.SearchCriteria) (bool, error) {
	e, err := m.entity()
	if err != nil {
		return false, err
	}
	return backendutil.Match(e, seqNum, m.Uid, m.Date, m.Flags, c)
}

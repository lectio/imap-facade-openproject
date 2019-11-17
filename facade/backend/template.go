package backend

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/emersion/go-imap"
	"github.com/jordan-wright/email"
	hal "github.com/lectio/go-json-hal"
	"github.com/spf13/viper"
)

var (
	templateFuncs = template.FuncMap{
		"url": func(url string) template.URL {
			return template.URL(url)
		},
	}
)

func AddTemplateFunc(key string, fn interface{}) {
	templateFuncs[key] = fn
}

type WorkPackageMessage struct {
	user *User
	// WorkPackage
	WorkPackage *hal.WorkPackage

	// Message
	Date    time.Time
	Subject string

	WordCount int
}

func (wpMsg *WorkPackageMessage) ReadingTime() string {
	mins := ReadingTime(wpMsg.WordCount)
	return fmt.Sprintf("%d minute read", int(mins.Minutes()))
}

func (wpMsg *WorkPackageMessage) Description(format string) interface{} {
	desc := wpMsg.WorkPackage.Description()
	if desc != nil {
		if format == "html" {
			return template.HTML(desc.Html)
		} else if format == "text" {
			return desc.Raw
		}
	}
	return ""
}

type EmailTemplate struct {
	template.Template
}

func NewEmailTemplate(cfg *viper.Viper) (*EmailTemplate, error) {
	cfgTpl := cfg.Sub("template")
	t := template.New("email").Funcs(sprig.FuncMap()).Funcs(templateFuncs)
	t.Funcs(template.FuncMap{
		"base": func() string {
			return cfg.GetString("base")
		},
	})

	files := cfgTpl.GetString("files")
	if _, err := t.ParseGlob(files); err != nil {
		return nil, err
	}
	tpl := &EmailTemplate{
		*t,
	}
	return tpl, nil
}

func (tpl *EmailTemplate) generatePart(name string, wpMsg *WorkPackageMessage) ([]byte, error) {
	var b bytes.Buffer
	if err := tpl.ExecuteTemplate(&b, name+".tpl", wpMsg); err != nil {
		log.Printf("Failed to generate '%s' part of work package email: %v", name, err)
		return nil, err
	}
	return b.Bytes(), nil
}

func (tpl *EmailTemplate) Generate(user *User, w *hal.WorkPackage) (*Message, error) {
	wpMsg := &WorkPackageMessage{
		user:        user,
		WorkPackage: w,
	}

	flags := []string{}
	// Build message
	e := email.NewEmail()

	// Calculate 'Date' for message
	date := time.Now()
	if dt := w.GetCreatedAt(); dt != nil {
		date = *dt
	}
	if dt := w.GetUpdatedAt(); dt != nil {
		date = *dt
	}
	wpMsg.Date = date
	e.Headers.Add("Date", date.Format(time.RFC1123Z))

	// From, To, CC
	from, _ := wpMsg.user.getCachedAddress(w.GetLink("author"))
	to, _ := wpMsg.user.getCachedAddress(w.GetLink("assignee"))
	cc, _ := wpMsg.user.getCachedAddress(w.GetLink("responsible"))

	e.From = from
	if to != "" {
		e.To = []string{to}
	}
	if cc != "" {
		e.Cc = []string{cc}
	}

	// Subject
	subject := w.Subject()
	// Check for Important marker `!1`
	if strings.HasSuffix(subject, " !1") {
		subject = strings.TrimSuffix(subject, " !1")
		flags = append(flags, imap.FlaggedFlag, "Important")
	}
	wpMsg.Subject = subject
	e.Subject = subject

	// Estimate reading time based on description word count
	if desc := w.Description(); desc != nil {
		// Based on: http://www.craigabbott.co.uk/how-to-calculate-reading-time-like-medium
		wpMsg.WordCount = WordCount(desc.Raw)
	}

	// Generate text & html parts
	if data, err := tpl.generatePart("html", wpMsg); err != nil {
		return nil, err
	} else {
		e.HTML = data
	}
	if data, err := tpl.generatePart("text", wpMsg); err != nil {
		return nil, err
	} else {
		e.Text = data
	}

	// Add attachments
	if attachments := w.GetAttachments(wpMsg.user.hal); attachments != nil {
		for _, res := range attachments.Items() {
			atRes, ok := res.(*hal.Attachment)
			if !ok {
				log.Printf("Invalid attachment=%+v", res)
			}
			reader, err := wpMsg.user.LoadAttachment(atRes)
			if err != nil {
				log.Printf("Failed to download attachment: %+v, err=%v", atRes, err)
				continue
			}
			if _, err := e.Attach(reader, atRes.FileName(), atRes.ContentType()); err != nil {
				log.Printf("Failed to add attachment: %v", err)
			}
		}
	}

	buf, err := e.Bytes()
	if err != nil {
		log.Printf("Failed to build message: subject=%s, err=%s", w.Subject(), err)
		return nil, err
	}

	msg := &Message{
		Date:          date,
		Flags:         flags,
		Size:          uint32(len(buf)),
		WorkPackageID: w.Id(),
		body:          buf,
		WordCount:     wpMsg.WordCount,
	}

	// Try loading flags stored in OpenProject
	wpMsg.user.loadWorkPackageFlags(msg)

	return msg, nil
}

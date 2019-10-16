package backend

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"time"

	"github.com/asdine/storm"
	hal "github.com/lectio/go-json-hal"
	"github.com/spf13/viper"
)

var (
	nameExpire = time.Second * 10
)

type Profile struct {
	Id    int
	Name  string
	Email string

	NameAndAddress string

	LastUpdated time.Time
}

func (u *Profile) checkExpire() bool {
	return u.LastUpdated.Add(nameExpire).Before(time.Now())
}

func (c *Cache) LoadCachedAddress(hc *hal.HalClient, link *hal.Link) (string, error) {
	if link == nil || link.Href == "" {
		return "", nil
	}

	// try getting from cache
	profile := Profile{}
	if err := c.db.Get("profiles", link.Href, &profile); err == nil {
		// Check if it has expired.
		if !profile.checkExpire() {
			// still valid.
			return profile.NameAndAddress, nil
		}
	}

	// request User resource
	res, err := hc.LinkGet(link)
	if err != nil {
		return "", err
	}
	userRes, ok := res.(*hal.User)
	if !ok {
		return "", fmt.Errorf("Invalid resource: %v", res)
	}

	// create/update cached profile
	profile.Id = userRes.Id()
	profile.Name = userRes.Name()
	profile.Email = userRes.Email()
	profile.NameAndAddress = formatEmailAddress(userRes)
	profile.LastUpdated = time.Now()
	if err := c.db.Set("profiles", link.Href, profile); err != nil {
		return "", fmt.Errorf("Failed to cache user profile: %v", err)
	}

	return profile.NameAndAddress, nil
}

func (c *Cache) LoadAttachment(hc *hal.HalClient, at *hal.Attachment) (io.Reader, error) {
	link := at.GetLink("downloadLocation")
	if link == nil || link.Href == "" {
		return nil, fmt.Errorf("Missing download link for attachment: %+v", at)
	}
	// Check for cached attachment
	if buf, err := c.db.GetBytes("attachments", link.Href); err == nil {
		return bytes.NewReader(buf), nil
	}
	// Download attachment
	atReader, err := at.Download(hc)
	if err != nil {
		log.Printf("Failed to download attachment: %+v, err=%v", at, err)
		return nil, err
	}
	// Read attachment into byte array
	buf, err := ioutil.ReadAll(atReader)
	if err != nil {
		log.Printf("Error reading attachment: %+v, err=%v", at, err)
		return nil, err
	}
	// Cache attachment
	if err := c.db.SetBytes("attachments", link.Href, buf); err != nil {
		log.Println("Failed to cache attachment:", err)
	}

	return bytes.NewReader(buf), nil
}

type Cache struct {
	db *storm.DB
}

func (c *Cache) Close() {
	if c.db != nil {
		c.db.Close()
		c.db = nil
	}
}

func (c *Cache) GetDB() *storm.DB {
	return c.db
}

func (c *Cache) GetNode(name string) storm.Node {
	return c.db.From(name)
}

func NewCache(cfg *viper.Viper) *Cache {
	if cfg == nil {
		log.Fatal("Missing cache settings.")
	}
	// global cache settings.
	nameExpire = time.Duration(cfg.GetInt("nameExpire")) * time.Second

	cache := &Cache{}

	// Open boltdb
	file := cfg.GetString("db")
	if db, err := storm.Open(file); err != nil {
		log.Fatal("Failed to open cache db:", err)
		return nil
	} else {
		cache.db = db
	}

	return cache
}

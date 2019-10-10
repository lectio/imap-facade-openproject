package backend

import (
	"fmt"
	"sync"
	"time"

	hal "github.com/lectio/go-json-hal"
	"github.com/spf13/viper"
)

var (
	nameExpire = time.Second * 10
)

type cacheUser struct {
	Id    int
	Name  string
	Email string

	NameAndAddress string

	lastUpdated time.Time
}

func (u *cacheUser) checkExpire() bool {
	return u.lastUpdated.Add(nameExpire).Before(time.Now())
}

type cacheUsers struct {
	sync.RWMutex

	cache map[string]*cacheUser
}

func newCacheUsers() *cacheUsers {
	return &cacheUsers{
		cache: make(map[string]*cacheUser),
	}
}

func (c *cacheUsers) getCachedAddress(link *hal.Link) (string, bool) {
	c.RLock()
	defer c.RUnlock()
	if user, ok := c.cache[link.Href]; ok {
		if user.checkExpire() {
			return "", false
		}
		return user.NameAndAddress, true
	}
	return "", false
}

func (c *cacheUsers) cacheUser(link *hal.Link, user *cacheUser) {
	c.Lock()
	defer c.Unlock()

	user.lastUpdated = time.Now()
	c.cache[link.Href] = user
}

func (c *cacheUsers) LoadCachedAddress(hc *hal.HalClient, link *hal.Link) (string, error) {
	if link == nil || link.Href == "" {
		return "", nil
	}

	// try getting from cache
	if addr, ok := c.getCachedAddress(link); ok {
		return addr, nil
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

	// Cache new user
	user := &cacheUser{
		Id:             userRes.Id(),
		Name:           userRes.Name(),
		Email:          userRes.Email(),
		NameAndAddress: formatEmailAddress(userRes),
	}
	c.cacheUser(link, user)
	return user.NameAndAddress, nil
}

type cachedObjects struct {
	sync.RWMutex

	cacheUserDetails *cacheUsers
}

func newCachedObjects() *cachedObjects {
	return &cachedObjects{
		cacheUserDetails: newCacheUsers(),
	}
}

func (c *cachedObjects) LoadCachedAddress(hc *hal.HalClient, link *hal.Link) (string, error) {
	return c.cacheUserDetails.LoadCachedAddress(hc, link)
}

func InitCache(cfg *viper.Viper) {
	nameExpire = time.Duration(cfg.GetInt("nameExpire")) * time.Second
}

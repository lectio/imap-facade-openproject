package facade

import (
	"crypto/tls"
	"log"
	"net/http"

	"github.com/spf13/viper"

	"golang.org/x/crypto/acme/autocert"
)

var (
	tlsEnabled = false
	tlsConfig  *tls.Config
)

func InitTLS(cfg *viper.Viper) error {
	tlsEnabled = cfg.GetBool("enabled")
	if !tlsEnabled {
		return nil
	}

	path := cfg.GetString("path")
	mode := cfg.GetString("mode")
	if mode != "autocert" {
		log.Println("-------- TODO: manual cert loading.")
		tlsEnabled = false
		return nil
	}

	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
	}
	if path != "" {
		m.Cache = autocert.DirCache(path)
	}
	hosts := cfg.GetStringSlice("hosts")
	if hosts != nil && len(hosts) > 0 {
		m.HostPolicy = autocert.HostWhitelist(hosts...)
	}
	tlsConfig = m.TLSConfig()
	addr := cfg.GetString("address")
	s := &http.Server{
		Addr:      addr,
		TLSConfig: tlsConfig,
	}
	go (func() {
		log.Printf("Start Autocert HTTPServer: %s", addr)
		err := s.ListenAndServeTLS("", "")
		log.Fatal(err)
	})()
	return nil
}

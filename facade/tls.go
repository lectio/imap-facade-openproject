package facade

import (
	"crypto/tls"
	"log"
	"net/http"

	"github.com/spf13/viper"

	"github.com/foomo/simplecert"
	"github.com/foomo/tlsconfig"
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

	auto := cfg.GetBool("auto")
	if !auto {
		log.Println("-------- TODO: manual cert loading.")
		tlsEnabled = false
		return nil
	}

	// Setup simplecert
	scfg := simplecert.Default
	scfg.CacheDir = cfg.GetString("path")
	scfg.Domains = cfg.GetStringSlice("hosts")
	scfg.SSLEmail = cfg.GetString("email")
	scfg.DNSProvider = cfg.GetString("dnsProvider")
	if cfg.GetBool("local") {
		scfg.Local = true
		// Don't update /etc/hosts by default
		scfg.UpdateHosts = false
	}
	if cfg.GetBool("updateHosts") {
		scfg.UpdateHosts = true
	}
	httpAddress := cfg.GetString("httpAddress")
	if httpAddress == "" {
		httpAddress = ":80"
		scfg.HTTPAddress = httpAddress
	}
	tlsAddress := cfg.GetString("httpsAddress")
	if tlsAddress == "" {
		tlsAddress = ":443"
		scfg.TLSAddress = tlsAddress
	}

	certReloader, err := simplecert.Init(scfg, nil)
	if err != nil {
		return err
	}

	// redirect HTTP to HTTPS
	// CAUTION: This has to be done AFTER simplecert setup
	// Otherwise Port 80 will be blocked and cert registration fails!
	log.Println("starting HTTP Listener on: ", httpAddress)
	go http.ListenAndServe(httpAddress, http.HandlerFunc(simplecert.Redirect))

	// init strict tlsConfig with certReloader
	// you could also use a default &tls.Config{}, but be warned this is highly insecure
	mode := cfg.GetString("mode")
	tlsConfig = tlsconfig.NewServerTLSConfig(tlsconfig.TLSModeServer(mode))

	// now set GetCertificate to the reloaders GetCertificateFunc to enable hot reload
	tlsConfig.GetCertificate = certReloader.GetCertificateFunc()

	// init server
	s := &http.Server{
		Addr:      tlsAddress,
		TLSConfig: tlsConfig,
	}

	log.Printf("Start Autocert HTTPServer: %s", tlsAddress)
	go startTLS(s)
	return nil
}

func startTLS(s *http.Server) {
	err := s.ListenAndServeTLS("", "")
	if err != nil {
		log.Fatal(err)
	}
}

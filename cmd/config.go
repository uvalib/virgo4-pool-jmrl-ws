package main

import (
	"flag"
	"log"
)

// ServiceConfig defines all of the JRML pool configuration parameters
type ServiceConfig struct {
	API       string
	APIKey    string
	APISecret string
	Port      int
}

// LoadConfiguration will load the service configuration from env/cmdline
// and return a pointer to it. Any failures are fatal.
func LoadConfiguration() *ServiceConfig {
	log.Printf("Loading configuration...")
	var cfg ServiceConfig
	flag.IntVar(&cfg.Port, "port", 8080, "JRML pool service port (default 8080)")
	flag.StringVar(&cfg.API, "api", "", "JRML API URL")
	flag.StringVar(&cfg.APIKey, "apikey", "", "Key you access the JRML API")
	flag.StringVar(&cfg.APISecret, "apisecret", "", "Secret to access the JRML API")

	flag.Parse()

	if cfg.API == "" {
		log.Fatal("Parameter -api is required")
	}
	if cfg.APIKey == "" {
		log.Fatal("Parameter -apikey is required")
	}
	if cfg.APISecret == "" {
		log.Fatal("Parameter -apisecret is required")
	}

	return &cfg
}

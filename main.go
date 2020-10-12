package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strconv"
)

// CatalogConfig stores the config parameters for the
// Catalog Worker
type CatalogConfig struct {
	Debug                 bool   // Enable extra logging
	URL                   string // The URL to your Ansible Tower
	Token                 string // The Token used to authenticate with Ansible Tower
	SkipVerifyCertificate bool   // Skip Certifcate Validation
}

func main() {
	rh := &DefaultRequestHandler{}
	startRun(os.Stdin, rh)
}

func startRun(reader io.Reader, rh RequestHandler) {

	config := CatalogConfig{}
	setConfig(&config)
	logFileName := "/tmp/catalog_worker_" + strconv.Itoa(os.Getpid()) + ".log"
	logf, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("error opening log file: %v", err)
	}
	defer logf.Close()
	defer log.Info("Finished Catalog Worker")

	configLogger(&config, logf)
	b, err := rh.getRequest(reader)
	if err != nil {
		log.Fatalf("Error getting request data %v", err)
	}
	log.Debug("Parsing incoming request")
	req, err := rh.parseRequest(b)
	if err != nil {
		log.Fatalf("Error parsing request %v", err)
	}

	log.Debug("Processing request")
	apiw := APIWorker{}
	rh.processRequest(req, config, &apiw)
}

func setConfig(config *CatalogConfig) {
	flag.StringVar(&config.Token, "token", "", "Ansible Tower token")
	flag.StringVar(&config.URL, "url", "", "Ansible Tower URL")
	flag.BoolVar(&config.Debug, "debug", false, "log debug messages")
	flag.BoolVar(&config.SkipVerifyCertificate, "skip_verify_ssl", false, "skip tower certificate verification")

	flag.Parse()
	if config.Token == "" || config.URL == "" {
		log.Fatal("Token and URL parameters are required")
	}
}

// Configure the logger
func configLogger(config *CatalogConfig, f *os.File) {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(f)
	if config.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
	log.SetReportCaller(true)
}

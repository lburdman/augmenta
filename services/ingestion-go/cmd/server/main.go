package main

import (
	"log"
	"net/http"
	"os"

	internalhttp "github.com/lburdman/augmenta/services/ingestion-go/internal/http"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/privacy"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
	"gopkg.in/yaml.v3"
)

func main() {
	log.SetFlags(0) // Remove default timestamp prefix from stdlib logger if we format manually, though standard is fine

	// Read flow config
	configPath := os.Getenv("FLOWS_CONFIG_PATH")
	if configPath == "" {
		configPath = "/configs/flows.yaml"
	}

	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("failed to read configuration file at %s: %v", configPath, err)
	}

	var cfg types.ConfigFile
	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		log.Fatalf("failed to parse yaml configuration: %v", err)
	}

	log.Printf("Loaded %d flows from config", len(cfg.Flows))

	privacyURL := os.Getenv("PRIVACY_SERVICE_URL")
	if privacyURL == "" {
		privacyURL = "http://privacy-service:8000"
	}

	downstreamURL := os.Getenv("DOWNSTREAM_MOCK_URL")
	if downstreamURL == "" {
		downstreamURL = "http://downstream-mock:9000"
	}

	client := privacy.NewClient(privacyURL, downstreamURL)
	server := internalhttp.NewServer(cfg.Flows, client)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Ingestion Service on port %s", port)
	if err := http.ListenAndServe(":"+port, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"

	internalhttp "github.com/lburdman/augmenta/services/ingestion-go/internal/http"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/privacy"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/vault"
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

	llmGatewayURL := os.Getenv("LLM_GATEWAY_URL")
	if llmGatewayURL == "" {
		llmGatewayURL = "http://llm-gateway-go:7001"
	}

	client := privacy.NewClient(privacyURL, llmGatewayURL)

	// Vault Initialization
	dynamoEndpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if dynamoEndpoint == "" {
		dynamoEndpoint = "http://dynamodb:8000"
	}
	vaultTableName := os.Getenv("VAULT_TABLE")
	if vaultTableName == "" {
		vaultTableName = "augmenta_vault"
	}

	var vlt vault.Vault
	if os.Getenv("VAULT_BACKEND") == "dynamodb" {
		log.Println("Initializing DynamoDB Vault...")
		v, err := vault.NewDynamoVault(context.Background(), dynamoEndpoint, vaultTableName)
		if err != nil {
			log.Fatalf("Failed to initialize vault: %v", err)
		}
		vlt = v
	}

	server := internalhttp.NewServer(cfg.Flows, client, vlt)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Ingestion Service on port %s", port)
	if err := http.ListenAndServe(":"+port, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

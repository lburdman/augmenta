package main

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	internalhttp "github.com/lburdman/augmenta/services/ingestion-go/internal/http"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/audit"
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
	privacyTimeoutStr := os.Getenv("PRIVACY_TIMEOUT_MS")
	privacyTimeout := 2000 // default 2s
	if privacyTimeoutStr != "" {
		if val, err := strconv.Atoi(privacyTimeoutStr); err == nil {
			privacyTimeout = val
		}
	}

	llmGatewayURL := os.Getenv("LLM_GATEWAY_URL")
	if llmGatewayURL == "" {
		llmGatewayURL = "http://llm-gateway-go:7001"
	}
	llmTimeoutStr := os.Getenv("LLM_TIMEOUT_MS")
	llmTimeout := 2000 // default 2s
	if llmTimeoutStr != "" {
		if val, err := strconv.Atoi(llmTimeoutStr); err == nil {
			llmTimeout = val
		}
	}

	client := privacy.NewClient(privacyURL, llmGatewayURL, time.Duration(privacyTimeout)*time.Millisecond, time.Duration(llmTimeout)*time.Millisecond)

	// Vault Initialization
	dynamoEndpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if dynamoEndpoint == "" {
		dynamoEndpoint = "http://dynamodb:8000"
	}
	vaultTableName := os.Getenv("VAULT_TABLE")
	if vaultTableName == "" {
		vaultTableName = "augmenta_vault"
	}
	vaultTimeoutStr := os.Getenv("VAULT_TIMEOUT_MS")
	vaultTimeout := 0 // default unlimited context
	if vaultTimeoutStr != "" {
		if val, err := strconv.Atoi(vaultTimeoutStr); err == nil {
			vaultTimeout = val
		}
	}

	var vlt vault.Vault
	if os.Getenv("VAULT_BACKEND") == "dynamodb" {
		log.Println("Initializing DynamoDB Vault...")

		encMode := os.Getenv("VAULT_ENCRYPTION_MODE")
		if encMode == "" {
			encMode = "dev"
		}

		masterKeyB64 := os.Getenv("VAULT_MASTER_KEY_B64")
		if masterKeyB64 == "" {
			log.Fatalf("VAULT_MASTER_KEY_B64 is required for vault encryption")
		}
		masterKey, err := base64.StdEncoding.DecodeString(masterKeyB64)
		if err != nil || len(masterKey) != 32 {
			log.Fatalf("VAULT_MASTER_KEY_B64 must be a valid 32-byte base64 string")
		}

		keysTable := os.Getenv("VAULT_KEYS_TABLE")
		if keysTable == "" {
			keysTable = "augmenta_vault_keys"
		}
		itemsTable := os.Getenv("VAULT_ITEMS_TABLE")
		if itemsTable == "" {
			itemsTable = "augmenta_vault_items"
		}

		v, err := vault.NewDynamoVault(context.Background(), dynamoEndpoint, keysTable, itemsTable, time.Duration(vaultTimeout)*time.Millisecond, masterKey, encMode)
		if err != nil {
			log.Fatalf("Failed to initialize vault: %v", err)
		}
		vlt = v
	}

	var auditLogger audit.Logger
	if os.Getenv("AUDIT_ADMIN_ENABLED") == "true" {
		log.Println("Initializing Audit In-Memory Ring Buffer Logger (Dev Only)...")
		auditLogger = audit.NewRingBufferLogger(200)
	}

	server := internalhttp.NewServer(cfg.Flows, client, vlt, auditLogger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Ingestion Service on port %s", port)
	if err := http.ListenAndServe(":"+port, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

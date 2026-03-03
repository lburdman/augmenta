package vault

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dty "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/crypto"
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
)

// VaultKeyItem stores the wrapped DEK per request
type VaultKeyItem struct {
	PK         string `dynamodbav:"pk"`
	SK         string `dynamodbav:"sk"`
	WrappedDEK []byte `dynamodbav:"wrapped_dek"`
	WrapAlg    string `dynamodbav:"wrap_alg"`
	CreatedAt  int64  `dynamodbav:"createdAt"`
	ExpiresAt  int64  `dynamodbav:"expiresAt"`
}

// VaultItem stores the encrypted entity mapping
type VaultItem struct {
	PK         string `dynamodbav:"pk"`
	SK         string `dynamodbav:"sk"`
	EntityType string `dynamodbav:"entityType"`
	Ciphertext []byte `dynamodbav:"ciphertext"`
	Nonce      []byte `dynamodbav:"nonce"`
	CreatedAt  int64  `dynamodbav:"createdAt"`
	ExpiresAt  int64  `dynamodbav:"expiresAt"`
}

type DynamoVault struct {
	client     *dynamodb.Client
	keysTable  string
	itemsTable string
	timeout    time.Duration
	masterKey  []byte
	encMode    string
}

func initClient(endpointURL string) *dynamodb.Client {
	// For Local DynamoDB we use dummy credentials and a custom endpoint
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpointURL != "" {
			o.BaseEndpoint = aws.String(endpointURL)
		}
	})
}

// NewDynamoVault initializes the DynamoDB client and ensures the dual tables exist.
func NewDynamoVault(ctx context.Context, endpointURL, keysTable, itemsTable string, timeout time.Duration, masterKey []byte, encMode string) (*DynamoVault, error) {
	client := initClient(endpointURL)
	v := &DynamoVault{
		client:     client,
		keysTable:  keysTable,
		itemsTable: itemsTable,
		timeout:    timeout,
		masterKey:  masterKey,
		encMode:    encMode,
	}
	
	if err := v.ensureTableExists(ctx, v.keysTable); err != nil {
		return nil, fmt.Errorf("failed to ensure keys table exists: %w", err)
	}
	if err := v.ensureTableExists(ctx, v.itemsTable); err != nil {
		return nil, fmt.Errorf("failed to ensure items table exists: %w", err)
	}
	
	return v, nil
}

func (v *DynamoVault) ensureTableExists(ctx context.Context, tableName string) error {
	_, err := v.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	
	if err == nil {
		// Table exists
		log.Printf("DynamoDB table %q already exists.", tableName)
		return nil
	}

	// Assuming the error is largely ResourceNotFoundException or similar, we proceed to create table
	log.Printf("Creating DynamoDB table %q...", tableName)
	_, err = v.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []dty.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: dty.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: dty.ScalarAttributeTypeS},
		},
		KeySchema: []dty.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: dty.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: dty.KeyTypeRange},
		},
		BillingMode: dty.BillingModePayPerRequest,
	})
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Wait for table to become active
	waiter := dynamodb.NewTableExistsWaiter(v.client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 20*time.Second)
	if err != nil {
		return fmt.Errorf("wait for table exists failed: %w", err)
	}

	log.Printf("DynamoDB table %q successfully created.", tableName)
	return nil
}

// getNow returns the current unix timestamp. Overridable for tests.
func getNow() int64 {
	override := os.Getenv("VAULT_NOW_OVERRIDE")
	if override != "" {
		if val, err := strconv.ParseInt(override, 10, 64); err == nil {
			return val
		}
	}
	return time.Now().Unix()
}

func (v *DynamoVault) PutMappings(ctx context.Context, tenantID, requestID string, ttlSeconds int, mappings []types.EntityMapping) error {
	if len(mappings) == 0 {
		return nil
	}
	
	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}

	now := getNow()
	expiresAt := now + int64(ttlSeconds)
	pk := fmt.Sprintf("TENANT#%s#REQ#%s", tenantID, requestID)

	// 1. Generate DEK and wrap it
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return fmt.Errorf("generate dek failed: %w", err)
	}

	var wrappedDEK []byte
	var wrapAlg string

	if v.encMode == "dev" {
		wrappedDEK, err = crypto.WrapDEK_DEV(v.masterKey, dek)
		if err != nil {
			return fmt.Errorf("wrap dek failed: %w", err)
		}
		wrapAlg = "DEV_AESGCM"
	} else {
		return fmt.Errorf("unsupported encryption mode: %s", v.encMode)
	}

	keyItem := VaultKeyItem{
		PK:         pk,
		SK:         "KEY#DEK",
		WrappedDEK: wrappedDEK,
		WrapAlg:    wrapAlg,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}

	keyAv, err := attributevalue.MarshalMap(keyItem)
	if err != nil {
		return fmt.Errorf("marshal key item error: %w", err)
	}

	var writeReqs []dty.WriteRequest
	writeReqs = append(writeReqs, dty.WriteRequest{
		PutRequest: &dty.PutRequest{Item: keyAv},
	})

	for _, m := range mappings {
		aad := []byte(fmt.Sprintf("tenantId=%s|requestId=%s|token=%s|entityType=%s", tenantID, requestID, m.Token, m.EntityType))
		nonce, ciphertext, err := crypto.EncryptValue(dek, []byte(m.Original), aad)
		if err != nil {
			return fmt.Errorf("encrypt value failed: %w", err)
		}

		item := VaultItem{
			PK:         pk,
			SK:         fmt.Sprintf("TOKEN#%s", m.Token),
			EntityType: m.EntityType,
			Ciphertext: ciphertext,
			Nonce:      nonce,
			CreatedAt:  now,
			ExpiresAt:  expiresAt,
		}

		av, err := attributevalue.MarshalMap(item)
		if err != nil {
			return fmt.Errorf("marshal item error: %w", err)
		}

		writeReqs = append(writeReqs, dty.WriteRequest{
			PutRequest: &dty.PutRequest{Item: av},
		})
	}

	// We might have more than 25 items total, so chunking is strictly needed.
	// For this Phase 4B PoC we do a basic loop over chunks of 25.
	for i := 0; i < len(writeReqs); i += 25 {
		end := i + 25
		if end > len(writeReqs) {
			end = len(writeReqs)
		}

		batch := writeReqs[i:end]
		// Determine which table gets which items:
		// We can just dump them all into a generic batch request if we mapped table targets correctly.
		// ACTUALLY, BatchWriteItem requires mapping By TableName.
		
		tableReqs := make(map[string][]dty.WriteRequest)
		for _, req := range batch {
			// Check SK prefix
			skAttr, ok := req.PutRequest.Item["sk"].(*dty.AttributeValueMemberS)
			if ok && skAttr.Value == "KEY#DEK" {
				tableReqs[v.keysTable] = append(tableReqs[v.keysTable], req)
			} else {
				tableReqs[v.itemsTable] = append(tableReqs[v.itemsTable], req)
			}
		}

		_, err := v.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: tableReqs,
		})
		if err != nil {
			return fmt.Errorf("batch write failed on chunk: %w", err)
		}
	}

	return nil
}

func (v *DynamoVault) GetOriginal(ctx context.Context, tenantID, requestID, token string) (string, error) {
	pk := fmt.Sprintf("TENANT#%s#REQ#%s", tenantID, requestID)
	skKey := "KEY#DEK"
	skToken := fmt.Sprintf("TOKEN#%s", token)

	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}

	// 1. Fetch the wrapped DEK
	keyOut, err := v.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(v.keysTable),
		Key: map[string]dty.AttributeValue{
			"pk": &dty.AttributeValueMemberS{Value: pk},
			"sk": &dty.AttributeValueMemberS{Value: skKey},
		},
	})
	if err != nil {
		return "", fmt.Errorf("get key item failed: %w", err)
	}
	if keyOut.Item == nil {
		return "", fmt.Errorf("vault dek missing")
	}

	var keyItem VaultKeyItem
	if err := attributevalue.UnmarshalMap(keyOut.Item, &keyItem); err != nil {
		return "", fmt.Errorf("unmarshal key item failed: %w", err)
	}

	// Wait, check TTL
	if getNow() > keyItem.ExpiresAt {
		return "", fmt.Errorf("token expired")
	}

	var dek []byte
	if v.encMode == "dev" {
		dek, err = crypto.UnwrapDEK_DEV(v.masterKey, keyItem.WrappedDEK)
		if err != nil {
			return "", fmt.Errorf("unwrap dek failed: %w", err)
		}
	} else {
		return "", fmt.Errorf("unsupported encryption mode: %s", v.encMode)
	}

	// 2. Fetch the mapped Item
	itemOut, err := v.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(v.itemsTable),
		Key: map[string]dty.AttributeValue{
			"pk": &dty.AttributeValueMemberS{Value: pk},
			"sk": &dty.AttributeValueMemberS{Value: skToken},
		},
	})
	if err != nil {
		return "", fmt.Errorf("get mapped item failed: %w", err)
	}
	if itemOut.Item == nil {
		return "", nil // Not found (graceful miss)
	}

	var item VaultItem
	if err := attributevalue.UnmarshalMap(itemOut.Item, &item); err != nil {
		return "", fmt.Errorf("unmarshal map failed: %w", err)
	}

	if getNow() > item.ExpiresAt {
		return "", fmt.Errorf("token expired")
	}

	// 3. Decrypt Item
	aad := []byte(fmt.Sprintf("tenantId=%s|requestId=%s|token=%s|entityType=%s", tenantID, requestID, token, item.EntityType))
	plaintext, err := crypto.DecryptValue(dek, item.Nonce, item.Ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt item failed: %w", err)
	}

	return string(plaintext), nil
}

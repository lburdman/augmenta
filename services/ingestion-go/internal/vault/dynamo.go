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
	"github.com/lburdman/augmenta/services/ingestion-go/internal/types"
)

// VaultItem represents a single DynamoDB record
type VaultItem struct {
	PK         string `dynamodbav:"pk"`
	SK         string `dynamodbav:"sk"`
	EntityType string `dynamodbav:"entityType"`
	Original   string `dynamodbav:"original"`
	CreatedAt  int64  `dynamodbav:"createdAt"`
	ExpiresAt  int64  `dynamodbav:"expiresAt"`
}

type DynamoVault struct {
	client    *dynamodb.Client
	tableName string
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

// NewDynamoVault initializes the DynamoDB client and ensures the table exists.
func NewDynamoVault(ctx context.Context, endpointURL, tableName string) (*DynamoVault, error) {
	client := initClient(endpointURL)
	v := &DynamoVault{client: client, tableName: tableName}
	
	if err := v.ensureTableExists(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure table exists: %w", err)
	}
	
	return v, nil
}

func (v *DynamoVault) ensureTableExists(ctx context.Context) error {
	_, err := v.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(v.tableName),
	})
	
	if err == nil {
		// Table exists
		log.Printf("DynamoDB table %q already exists.", v.tableName)
		return nil
	}

	// Assuming the error is largely ResourceNotFoundException or similar, we proceed to create table
	log.Printf("Creating DynamoDB table %q...", v.tableName)
	_, err = v.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(v.tableName),
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
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(v.tableName)}, 20*time.Second)
	if err != nil {
		return fmt.Errorf("wait for table exists failed: %w", err)
	}

	log.Printf("DynamoDB table %q successfully created.", v.tableName)
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

	now := getNow()
	expiresAt := now + int64(ttlSeconds)
	pk := fmt.Sprintf("TENANT#%s#REQ#%s", tenantID, requestID)

	var writeReqs []dty.WriteRequest
	for _, m := range mappings {
		item := VaultItem{
			PK:         pk,
			SK:         fmt.Sprintf("TOKEN#%s", m.Token),
			EntityType: m.EntityType,
			Original:   m.Original,
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

	// Simple batch write (assuming < 25 items for demo). In production we'd chunk this.
	_, err := v.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]dty.WriteRequest{
			v.tableName: writeReqs,
		},
	})
	if err != nil {
		return fmt.Errorf("batch write failed: %w", err)
	}

	return nil
}

func (v *DynamoVault) GetOriginal(ctx context.Context, tenantID, requestID, token string) (string, error) {
	pk := fmt.Sprintf("TENANT#%s#REQ#%s", tenantID, requestID)
	sk := fmt.Sprintf("TOKEN#%s", token)

	out, err := v.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(v.tableName),
		Key: map[string]dty.AttributeValue{
			"pk": &dty.AttributeValueMemberS{Value: pk},
			"sk": &dty.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return "", fmt.Errorf("get item failed: %w", err)
	}

	if out.Item == nil {
		return "", nil // Not found
	}

	var item VaultItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return "", fmt.Errorf("unmarshal map failed: %w", err)
	}

	// Application-side TTL check
	if getNow() > item.ExpiresAt {
		return "", fmt.Errorf("token expired")
	}

	return item.Original, nil
}

# Augmenta Privacy Service Scaffold

A minimal proof-of-concept for detecting and anonymizing PII using Microsoft Presidio and FastAPI.

## Quickstart

- `make run`: Starts the services using Docker Compose.
- `make test-smoke`: Sends a sample curl request with PII and prints the anonymized output from `privacy-service`.
- `make test-integration`: Runs the End-to-End integration test via `ingestion-go`.
- `make test`: Runs `pytest` inside the `privacy-service` container for unit tests.
- `make down`: Stops the services.

## Architecture & Configuration

The application operates an ingestion webhook that immediately routes requests via a generic `privacy-service` for anonymization before forwarding them to an LLM Gateway for evaluation.

Routing configuration is defined in `/configs/flows.yaml`:
```yaml
flows:
  - tenantId: tenantA
    sourceId: demo
    operators:
      DEFAULT:
        type: replace
        new_value: "<REDACTED>"
```

## Vault Envelope Encryption (Phase 4B)

By default, the Vault operates via AWS DynamoDB using Envelope Encryption (AES-GCM). This secures tokenized `Original` strings from leaking inside underlying Datastores.

**Storage Mechanics**:
- `augmenta_vault_keys`: Dedicated table containing `wrapped_dek`. Key material is uniquely generated per `(TenantId, RequestId)`.
- `augmenta_vault_items`: Dedicated table containing the Nonce and AES-GCM cipher payloads bounding tokens correctly with AAD validations. 

**Local Configuration**:
During standard development modes (`VAULT_ENCRYPTION_MODE=dev`), the internal DEKs are wrapped using an AES-256 Master Key provided via Environment Variable. 

To run safely locally, use the provided generation script matching Compose bindings:
```bash
# Export the 32-byte B64 generated master key to your environment instance
export VAULT_MASTER_KEY_B64=$(./scripts/gen_dev_master_key.sh)
make run
```

## API Endpoints

### 1. Ingestion Service (Go)
**`POST http://localhost:8080/ingest/webhook/{sourceId}`**

- **Headers:** `X-Tenant-ID: <tenantId>`
- **Request Body:**
```json
{
  "text": "Contact me at john.doe@example.com"
}
```
- **Response:**
```json
{
  "requestId": "uuid",
  "tenantId": "tenantA",
  "sourceId": "demo",
  "anonymized_text": "Contact me at <REDACTED>",
  "llm_output": "ECHO: Contact me at <REDACTED>",
  "provider": "echo"
}
```

### 2. LLM Gateway Service (Go)
**`GET http://localhost:7001/last`**
- Intercepts and holds the forwarded anonymized requests securely. This endpoint is strictly for verifying that PII wasn't forwarded during tests.

### 3. Downstream Mock Service (Python)
**`GET http://localhost:9000/last`**
- Maintained as an alternative sink.
- **Swagger UI:** `http://localhost:9000/docs`

### 4. Admin Audit Access (Go plugin)
**`GET http://localhost:8080/admin/audit`** *(Optional)*
- Exposes an in-memory ring-buffer containing the 200 most recent structured JSON events covering Pipeline steps (`anonymize`, `vault_put`, `llm_call`, `rehydrate`).
- Contains `metrics` blocks but **No** PII, source text, or result outputs.
- Can be activated safely in pre-prod by injecting `AUDIT_ADMIN_ENABLED=true`.

### 5. Privacy Provider Service (Python)
**`POST http://localhost:8000/anonymize`**
- **Swagger UI:** `http://localhost:8000/docs`

**Request Schema:**
```json
{
  "requestId": "any-string",
  "tenantId": "any-string",
  "text": "string to anonymize",
  "operators": {
     "DEFAULT": { "type": "replace", "new_value": "<REDACTED>" }
  }
}
```

**Response Schema:**
```json
{
  "anonymized_text": "...",
  "analyzer_results": [
    { "entity_type": "EMAIL_ADDRESS", "start": 0, "end": 5, "score": 0.85 }
  ],
  "stats": {
    "entities_total": 1,
    "entities_by_type": { "EMAIL_ADDRESS": 1 }
  }
}
```

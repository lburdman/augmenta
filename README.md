# Augmenta Privacy Service Scaffold

A minimal proof-of-concept for detecting and anonymizing PII using Microsoft Presidio and FastAPI.

## Quickstart

- `make run`: Starts the service using Docker Compose.
- `make test-smoke`: Sends a sample curl request with PII and prints the anonymized output.
- `make test`: Runs `pytest` inside the container.
- `make down`: Stops the service.

## API Usage

### `POST /anonymize`

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

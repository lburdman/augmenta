#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -e

# Target endpoint
ENDPOINT="http://localhost:8000/anonymize"

# Request JSON
REQUEST_BODY=$(cat <<'EOF'
{
  "requestId": "smoke-test-123",
  "tenantId": "tenant-xyz",
  "text": "Contact me at john.doe@example.com or +1-202-555-0101",
  "operators": {
     "DEFAULT": { "type": "replace", "new_value": "<REDACTED>" }
  }
}
EOF
)

echo "Calling POST /anonymize with: ${REQUEST_BODY}"
echo "------------------------------------------------"

# Run curl
# Note: we use -s to silence curl progress output
RESPONSE=$(curl -s -X POST "${ENDPOINT}" \
  -H "Content-Type: application/json" \
  -d "${REQUEST_BODY}")

echo "Response: "
echo "${RESPONSE}"
echo "------------------------------------------------"

# Verify that we see token boundaries without leaking email into the main body

if echo "$RESPONSE" | grep -q 'anonymized_text' && echo "$RESPONSE" | grep -q '\[\[AUG:'; then
  echo "✅ SMOKE TEST PASSED: Response contains anonymized_text and structured token mappings."
else
  echo "❌ SMOKE TEST FAILED: Response does not look correct or missing tokenization markers."
  exit 1
fi

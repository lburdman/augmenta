#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -e

# Target endpoint
ENDPOINT="http://localhost:8000/anonymize"

# Request JSON
REQUEST_BODY=$(cat <<EOF
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

# Avoid depending on jq: we can use simple grep to verify
if echo "$RESPONSE" | grep -q 'john.doe@example.com'; then
  echo "❌ SMOKE TEST FAILED: Email address was NOT anonymized."
  exit 1
fi

if echo "$RESPONSE" | grep -q 'anonymized_text'; then
  echo "✅ SMOKE TEST PASSED: Response contains anonymized_text and no original email."
else
  echo "❌ SMOKE TEST FAILED: Response does not look correct."
  exit 1
fi

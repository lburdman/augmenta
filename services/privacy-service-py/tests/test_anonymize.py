from fastapi.testclient import TestClient
from app.main import app

client = TestClient(app)

def test_health():
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}

def test_anonymize_email_and_phone():
    payload = {
        "requestId": "test-req-1",
        "tenantId": "test-tenant-1",
        "text": "Hello, my email is jane.doe@example.com and my phone is +1-202-555-0101.",
        "operators": {
            "DEFAULT": { "type": "replace", "new_value": "<REDACTED>" }
        }
    }
    
    response = client.post("/anonymize", json=payload)
    assert response.status_code == 200
    
    data = response.json()
    
    # Verify the output schema matches expectations
    assert "anonymized_text" in data
    assert "analyzer_results" in data
    assert "stats" in data
    
    original_text = payload["text"]
    anonymized_text = data["anonymized_text"]
    
    # The anonymized text shouldn't contain the email
    assert "jane.doe@example.com" not in anonymized_text
    assert "[[AUG:EMAIL_ADDRESS:1]]" in anonymized_text
    
    # Check the analyzer results contain EMAIL_ADDRESS
    entity_types = [res["entity_type"] for res in data["analyzer_results"]]
    assert "EMAIL_ADDRESS" in entity_types
    
    assert data["stats"]["entities_total"] > 0
    assert data["stats"]["entities_by_type"].get("EMAIL_ADDRESS", 0) > 0

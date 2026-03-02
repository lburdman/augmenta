import logging
from fastapi import FastAPI
from pydantic import BaseModel

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("downstream-mock")

app = FastAPI(title="Downstream Mock Service")

# In-memory storage of the last requested payload
last_received_payload = None

class Payload(BaseModel):
    requestId: str
    tenantId: str
    sourceId: str
    anonymized_text: str

@app.post("/receive")
async def receive(payload: Payload):
    global last_received_payload
    last_received_payload = payload.model_dump()
    
    # Store request but ONLY log the requestId
    logger.info(f"Received payload successfully for requestId: {payload.requestId}")
    
    return {"status": "ok"}

@app.get("/last")
async def get_last():
    """Fetch the last received payload. Used ONLY for integration testing."""
    if not last_received_payload:
        return {"error": "No payload received yet."}
    return last_received_payload

@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "ok"}

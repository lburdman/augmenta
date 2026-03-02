import logging
import time
from typing import Dict, Any, List

from fastapi import FastAPI, Request
from pydantic import BaseModel, Field
from presidio_analyzer import AnalyzerEngine
from presidio_anonymizer import AnonymizerEngine
from presidio_anonymizer.entities import OperatorConfig

# Setup basic logging
# NEVER log the raw input text. Logs may include requestId, tenantId, entity counts, and latency only.
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("privacy-service")

app = FastAPI(title="Augmenta Privacy Service")

# Initialize Presidio Engines globally to avoid reloading models on every request
logger.info("Initializing Presidio Analyzer and Anonymizer engines...")
analyzer = AnalyzerEngine()
anonymizer = AnonymizerEngine()
logger.info("Engines initialized.")

class OperatorParams(BaseModel):
    type: str
    new_value: str

class AnonymizeRequest(BaseModel):
    requestId: str
    tenantId: str
    text: str
    operators: Dict[str, OperatorParams] = Field(
        default_factory=lambda: {"DEFAULT": OperatorParams(type="replace", new_value="<REDACTED>")}
    )

class AnalyzerResultDto(BaseModel):
    entity_type: str
    start: int
    end: int
    score: float

class StatsDto(BaseModel):
    entities_total: int
    entities_by_type: Dict[str, int]

class AnonymizeResponse(BaseModel):
    anonymized_text: str
    analyzer_results: List[AnalyzerResultDto]
    stats: StatsDto

@app.get("/health")
def health_check():
    return {"status": "ok"}

@app.post("/anonymize", response_model=AnonymizeResponse)
async def anonymize(req: AnonymizeRequest, request: Request):
    start_time = time.time()
    
    # 1. Analyze text for PII
    analyzer_results = analyzer.analyze(text=req.text, language="en")
    
    # 2. Configure operators
    # By default we map the provided operators to AnonymizerEngine OperatorConfig
    operators_config = {}
    for op_key, op_val in req.operators.items():
        operators_config[op_key] = OperatorConfig(op_val.type, {"new_value": op_val.new_value})
    
    # 3. Anonymize
    anonymizer_result = anonymizer.anonymize(
        text=req.text,
        analyzer_results=analyzer_results,
        operators=operators_config
    )
    
    # 4. Prepare statistics
    entities_by_type = {}
    for res in analyzer_results:
        entities_by_type[res.entity_type] = entities_by_type.get(res.entity_type, 0) + 1
        
    stats = StatsDto(
        entities_total=len(analyzer_results),
        entities_by_type=entities_by_type
    )
    
    # 5. Build analyzer results output
    results_dto = [
        AnalyzerResultDto(
            entity_type=res.entity_type, 
            start=res.start, 
            end=res.end, 
            score=res.score
        ) for res in analyzer_results
    ]
    
    latency = time.time() - start_time
    
    # Log securely (no PII text)
    logger.info(
        f"Anonymize Request - requestId: {req.requestId}, "
        f"tenantId: {req.tenantId}, "
        f"entities_total: {stats.entities_total}, "
        f"latency: {latency:.4f}s"
    )
    
    return AnonymizeResponse(
        anonymized_text=anonymizer_result.text,
        analyzer_results=results_dto,
        stats=stats
    )

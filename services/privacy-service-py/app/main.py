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
    type: str = "token"
    new_value: str = ""

class AnonymizeRequest(BaseModel):
    requestId: str
    tenantId: str
    text: str
    operators: Dict[str, OperatorParams] = Field(
        default_factory=lambda: {"DEFAULT": OperatorParams(type="token", new_value="")}
    )

class AnalyzerResultDto(BaseModel):
    entity_type: str
    start: int
    end: int
    score: float

class StatsDto(BaseModel):
    entities_total: int
    entities_by_type: Dict[str, int]

class EntityMappingDto(BaseModel):
    token: str
    entity_type: str
    original: str

class AnonymizeResponse(BaseModel):
    anonymized_text: str
    mappings: List[EntityMappingDto]
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
    
    # 2. Assign tokens and construct mapping
    # Sort by start position ascending, then end descending to process inner/outer correctly
    sorted_results = sorted(analyzer_results, key=lambda x: (x.start, -x.end))
    
    filtered_results = []
    last_end = -1
    for res in sorted_results:
        # Ignore overlapping spans to prevent token corruption
        if res.start >= last_end:
            filtered_results.append(res)
            last_end = res.end

    mappings = []
    mappings_dtos = []
    seq_counter = {}
    
    # Build list of mappings for replacement
    for res in filtered_results:
        seq = seq_counter.get(res.entity_type, 0) + 1
        seq_counter[res.entity_type] = seq
        
        token = f"[[AUG:{res.entity_type}:{seq}]]"
        original = req.text[res.start:res.end]
        
        mappings.append({
            "token": token,
            "entity_type": res.entity_type,
            "original": original,
            "start": res.start,
            "end": res.end
        })
        mappings_dtos.append(EntityMappingDto(token=token, entity_type=res.entity_type, original=original))
    
    # 3. Anonymize/Tokenize backwards to safely apply replacements
    anonymized_text = req.text
    for m in reversed(mappings):
        anonymized_text = anonymized_text[:m["start"]] + m["token"] + anonymized_text[m["end"]:]
    
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
        anonymized_text=anonymized_text,
        mappings=mappings_dtos,
        analyzer_results=results_dto,
        stats=stats
    )

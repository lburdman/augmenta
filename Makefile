.PHONY: run down test test-smoke test-integration

run:
	docker compose up --build -d

down:
	docker compose down

test:
	docker compose run --rm -e PYTHONPATH=/app privacy-service pytest tests/
	# Note: to run go unit tests, you would run cd services/ingestion-go && go test ./... locally

test-smoke:
	bash scripts/smoke_test.sh

test-integration:
	docker run --rm --network augmenta_default -v $(PWD)/services/ingestion-go:/app -w /app golang:1.23-alpine go test -v ./tests/...


.PHONY: run down test test-smoke

run:
	docker compose up --build -d

down:
	docker compose down

test:
	docker compose run --rm -e PYTHONPATH=/app privacy-service pytest tests/

test-smoke:
	bash scripts/smoke_test.sh

SERVICE ?= jobs
SERVICES := gateway jobs risk prevention

.PHONY: test build up down logs integration demo
test:
	@for service in $(SERVICES); do (cd services/$$service && GOWORK=off go test ./... && GOWORK=off go vet ./...) || exit 1; done
build:
	docker compose build $(SERVICE)
up:
	docker compose up -d --build
down:
	docker compose down
logs:
	docker compose logs -f $(SERVICE)
integration:
	docker compose --profile test run --build --rm jobs-test
	docker compose --profile test run --build --rm risk-test
	docker compose --profile test run --build --rm prevention-test
demo:
	python scripts/smoke.py

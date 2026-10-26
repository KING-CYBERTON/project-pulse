.PHONY: up down logs restart run test build

up:        ## build and start in the background, then open http://localhost:8080
	docker compose up -d --build
	@echo "Project Pulse: http://localhost:$${PULSE_PORT:-8080}"

down:      ## stop
	docker compose down

logs:
	docker compose logs -f

restart:   ## reload projects.json
	docker compose restart

run:       ## run without Docker (needs Go 1.24+); reads .env
	set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/pulse

test:
	go vet ./... && go test ./...

build:
	go build -o bin/pulse ./cmd/pulse

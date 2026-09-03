.PHONY: build test vet lint smoke doctor

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	sh scripts/check-provider-isolation.sh

smoke:
	docker compose up -d --build postgres redis nats tide-api tide-engine
	sleep 8
	curl -sf http://localhost:8080/healthz
	curl -sf http://localhost:8081/healthz
	go run ./cmd/tide simulate --vehicles 10 --scenario mixed

doctor:
	go run ./cmd/tide doctor --config tide.yaml.example

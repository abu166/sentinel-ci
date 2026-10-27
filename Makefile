.PHONY: setup build test test-race lint up docker-up seed simulate-bug clean integration
setup:
	cd services/gateway-api && go mod download
	cd services/runner-daemon && go mod download
	python3 -m venv services/healer-service/.venv
	services/healer-service/.venv/bin/pip install -r services/healer-service/requirements.txt
	cd web-console && npm install
build:
	cd services/gateway-api && go build ./...
	cd services/runner-daemon && go build ./...
	cd web-console && npm run build
test:
	cd services/gateway-api && go test -v ./...
	cd services/runner-daemon && go test -v ./...
	cd services/healer-service && .venv/bin/python -m pytest -v --cov=healer
	cd web-console && npm test -- --run
test-race:
	cd services/gateway-api && go test -v -race ./...
	cd services/runner-daemon && go test -v -race ./...
lint:
	cd services/gateway-api && go vet ./...
	cd services/runner-daemon && go vet ./...
	cd web-console && npx tsc --noEmit
	python3 -m compileall -q services/healer-service/healer
up: docker-up
docker-up:
	docker compose -f deploy/docker-compose.yml up --build -d --wait
seed:
	sh deploy/seed-demo-data.sh
simulate-bug:
	python3 e2e/simulate.py
integration:
	python3 e2e/phase2.py
smoke:
	python3 e2e/integration.py
clean:
	docker compose -f deploy/docker-compose.yml down

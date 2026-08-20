.PHONY: ui backend build dev test lint fmt clean

ui:
	cd web && yarn install --immutable && yarn build

backend:
	mkdir -p bin
	go build -o bin/qac ./cmd/qac

build: ui backend

dev:
	@echo "Go on :8080, Vite on :5173 (proxies /api → :8080)"
	@bash -c 'trap "kill 0" EXIT; \
	  go run ./cmd/qac serve --addr 127.0.0.1:8080 & \
	  (cd web && yarn dev) & \
	  wait'

test:
	go test ./...
	cd web && yarn test --run

lint:
	go vet ./...
	cd web && yarn lint

fmt:
	go fmt ./...
	cd web && yarn format

clean:
	rm -rf web/dist web/node_modules bin/qac

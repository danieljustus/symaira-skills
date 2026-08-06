.PHONY: build test lint fmt-check clean smoke coverage

build:
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(shell git describe --tags --always --dirty)" -o symskills ./cmd/symskills

test:
	CGO_ENABLED=0 go test -race ./...

coverage:
	CGO_ENABLED=0 go test ./... -coverprofile=coverage.out
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%", "", $$3); print $$3}'); \
		awk -v total="$$total" 'BEGIN { if (total < 75) { printf "coverage %.1f%% is below the 75%% repository gate\n", total; exit 1 } printf "coverage %.1f%% (minimum 75%%)\n", total }'

lint: fmt-check
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt diff found:" && gofmt -l . && exit 1)

# smoke — opt-in headless smoke test
# Installs a fixture skill into a temp HOME and drives each available harness
# headless, asserting the skill was loaded.  Skips cleanly when a harness
# binary is absent or the required API key is not configured.
smoke: build
	@echo "Running headless smoke test..."
	@./scripts/smoke-harness.sh

clean:
	go clean -cache -testcache
	rm -f symskills coverage.out
	rm -rf bin/ dist/

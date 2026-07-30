.PHONY: build test lint fmt-check clean smoke

build:
	CGO_ENABLED=0 go build -o symskills ./cmd/symskills

test:
	CGO_ENABLED=0 go test -race ./...

lint: fmt-check
	go vet ./...

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

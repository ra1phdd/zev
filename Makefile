GO ?= go
APP ?= ./cmd/zev
BIN ?= bin/zev
GOFMT ?= gofmt
GOFUMPT ?= gofumpt
GOIMPORTS ?= goimports
GOLANGCI_LINT ?= golangci-lint
DEADCODE ?= deadcode
GO_FILES := $(shell find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*')
FLAGS = -ldflags "-s -w -X main.version=$(VERSION) -buildid=" -trimpath
VERSION ?= dev

.DEFAULT_GOAL := help
.PHONY: help build clean test test-race test-integration test-e2e test-manual \
	lint fmt check-bin check-go-files check-format-tools check-lint-tools \
	install run setup build-linux build-darwin build-windows build-all \
	release-snapshot release-check release dev watch check version info

help: ## Показать это сообщение
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

check-bin:
	@case "$(BIN)" in bin/?*) ;; *) echo 'BIN must point inside bin/' >&2; exit 1 ;; esac
	@case "$(BIN)" in *..*|*/) echo 'BIN must name a file without parent traversal' >&2; exit 1 ;; esac

build: check-bin ## Собрать бинарник в BIN (по умолчанию bin/zev)
	mkdir -p "$(dir $(BIN))"
	$(GO) build $(FLAGS) -o "$(BIN)" $(APP)

clean: check-bin ## Удалить бинарник BIN и его кросс-платформенные сборки
	rm -f -- "$(BIN)" "$(BIN)-linux-amd64" "$(BIN)-darwin-amd64" "$(BIN)-darwin-arm64" "$(BIN)-windows-amd64.exe"

test: ## Запустить go vet и unit-тесты
	$(GO) vet ./...
	$(GO) test ./...

test-race: ## Запустить unit-тесты с детектором гонок
	$(GO) test -race ./...

test-integration: test-e2e ## Запустить совместимый integration/e2e gate

test-e2e: ## Запустить сквозные тесты из e2e/ с реальным бинарником
	$(GO) test -tags=e2e ./e2e/...

test-manual: ## Запустить manual-тесты только с ALLOW_MANUAL_TESTS=1 и вне CI
	@if [ -n "$${CI:-}" ]; then echo 'manual tests must not run in CI' >&2; exit 1; fi
	@test "$(ALLOW_MANUAL_TESTS)" = "1" || { echo 'manual tests require explicit permission: set ALLOW_MANUAL_TESTS=1' >&2; exit 1; }
	@test -n "$$(grep -R -l --include='*_test.go' '^//go:build .*manual\|^//go:build manual' . 2>/dev/null)" || { echo 'no manual-tagged tests are defined' >&2; exit 1; }
	$(GO) test -tags=manual ./...

lint: check-go-files check-lint-tools ## Проверить форматирование, применить исправления линтера и вывести deadcode
	@files="$$($(GOFMT) -l $(GO_FILES))" || exit $$?; test -z "$$files" || { printf '%s\n' "$$files"; echo 'gofmt required; run make fmt' >&2; exit 1; }
	@files="$$($(GOFUMPT) -l $(GO_FILES))" || exit $$?; test -z "$$files" || { printf '%s\n' "$$files"; echo 'gofumpt required; run make fmt' >&2; exit 1; }
	@files="$$($(GOIMPORTS) -l $(GO_FILES))" || exit $$?; test -z "$$files" || { printf '%s\n' "$$files"; echo 'goimports required; run make fmt' >&2; exit 1; }
	$(GOLANGCI_LINT) run --fix
	$(DEADCODE) -test $(APP)

fmt: check-go-files check-format-tools ## Применить gofmt, goimports и gofumpt
	$(GOFMT) -w $(GO_FILES)
	$(GOIMPORTS) -w $(GO_FILES)
	$(GOFUMPT) -w $(GO_FILES)

check-go-files:
	@test -n "$(GO_FILES)" || { echo 'no Go files found' >&2; exit 1; }

check-format-tools:
	@for tool in "$(GOFMT)" "$(GOFUMPT)" "$(GOIMPORTS)"; do command -v "$$tool" >/dev/null || { echo "required tool not found in PATH: $$tool" >&2; exit 1; }; done

check-lint-tools: check-format-tools
	@for tool in "$(GOLANGCI_LINT)" "$(DEADCODE)"; do command -v "$$tool" >/dev/null || { echo "required tool not found in PATH: $$tool" >&2; exit 1; }; done

install: ## Установить APP через go install (GOBIN или GOPATH/bin)
	$(GO) install $(FLAGS) $(APP)

run: build ## Собрать и запустить интерактивный режим (ARGS для аргументов)
	@"$(BIN)" $(ARGS)

setup: build ## Запустить интерактивную настройку
	@"$(BIN)" setup $(ARGS)

build-linux: ## Собрать для Linux amd64
	$(MAKE) build GOOS=linux GOARCH=amd64 BIN="$(BIN)-linux-amd64"

build-darwin: ## Собрать для macOS amd64 и arm64
	$(MAKE) build GOOS=darwin GOARCH=amd64 BIN="$(BIN)-darwin-amd64"
	$(MAKE) build GOOS=darwin GOARCH=arm64 BIN="$(BIN)-darwin-arm64"

build-windows: ## Собрать для Windows amd64
	$(MAKE) build GOOS=windows GOARCH=amd64 BIN="$(BIN)-windows-amd64.exe"

build-all: build-linux build-darwin build-windows ## Собрать для всех локальных платформенных целей

release-snapshot: ## Локальная проверка релизной сборки GoReleaser (без публикации)
	goreleaser release --snapshot --clean --skip=publish

release-check: ## Проверить .goreleaser.yaml без сборки
	goreleaser check

release: ## Опубликовать релиз только после всех локальных проверок
	@test -f VERSION || (echo "VERSION not found" && exit 1)
	@bash deploy/release.sh "$$(tr -d '[:space:]' < VERSION)"

dev: ## Запустить в режиме разработки (debug логи)
	$(GO) run $(FLAGS) $(APP) -vv $(ARGS)

watch: ## Запустить с автоперезапуском (требуется entr)
	@command -v entr >/dev/null || { echo 'required tool not found in PATH: entr' >&2; exit 1; }
	@find . -type f -name '*.go' -not -path './.git/*' -not -path './vendor/*' | entr -r $(MAKE) run

check: lint test ## Запустить проверки форматирования, линтер и unit-тесты

version: ## Показать версию сборки (VERSION, по умолчанию dev)
	@echo "$(VERSION)"

info: ## Показать информацию о проекте
	@echo "App:         $(APP)"
	@echo "Binary:      $(BIN)"
	@echo "Version:     $(VERSION)"
	@echo "Go version:  $$($(GO) version)"
	@echo "Platform:    $$($(GO) env GOOS)/$$($(GO) env GOARCH)"

.PHONY: help build test lint clean install run setup

# Версия приложения
VERSION ?= dev
BINARY_NAME = zev
BUILD_DIR = bin
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

help: ## Показать это сообщение
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Собрать бинарник
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/zev
	@echo "Binary created at $(BUILD_DIR)/$(BINARY_NAME)"

test: ## Запустить все тесты
	go test ./... -race

lint: ## Запустить линтер
	golangci-lint run --fix

clean: ## Очистить артефакты сборки
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	go clean

install: build ## Установить в GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) ./cmd/zev
	@echo "Installed to $$(go env GOPATH)/bin/$(BINARY_NAME)"

run: build ## Собрать и запустить в интерактивном режиме
	@$(BUILD_DIR)/$(BINARY_NAME)

setup: build ## Запустить интерактивную настройку
	@$(BUILD_DIR)/$(BINARY_NAME) setup

# Сборка для разных платформ
build-linux: ## Собрать для Linux
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/zev

build-darwin: ## Собрать для macOS
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/zev
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/zev

build-windows: ## Собрать для Windows
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/zev

build-all: build-linux build-darwin build-windows ## Собрать для всех платформ

# Разработка
dev: ## Запустить в режиме разработки (debug логи)
	@LOG_LEVEL=debug go run $(LDFLAGS) ./cmd/zev

watch: ## Запустить с автоперезапуском (требуется entr)
	@find . -name "*.go" | entr -r make run

# Проверки качества
check: lint test ## Запустить линтер и тесты

# Информация
version: ## Показать версию
	@echo $(VERSION)

info: ## Показать информацию о проекте
	@echo "Binary name: $(BINARY_NAME)"
	@echo "Version:     $(VERSION)"
	@echo "Build dir:   $(BUILD_DIR)"
	@echo "Go version:  $$(go version)"
	@echo "Platform:    $$(go env GOOS)/$$(go env GOARCH)"

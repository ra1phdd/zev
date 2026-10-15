# Zev — Natural Language to Shell Commands

Zev — это CLI-инструмент, который превращает естественное описание задачи в готовые команды терминала с использованием LLM.

## Установка

### Из исходников

```bash
go install github.com/ra1phdd/zev-go/cmd/zev@latest
```

### Локальная сборка

```bash
git clone https://github.com/ra1phdd/zev-go.git
cd zev-go
go build -o zev ./cmd/zev
sudo mv zev /usr/local/bin/
```

## Быстрый старт

### Первичная настройка

```bash
zev setup
```

Интерактивный мастер предложит выбрать LLM-провайдера и ввести необходимые параметры:

- **OpenAI**: требуется API key, можно указать custom base URL для совместимых API
- **Gemini**: требуется Google API key
- **Ollama**: требуется локальный URL (например, `http://localhost:11434/v1`)

Конфигурация сохраняется в `~/.zevrc` (основной файл) и `~/.zevrc.security.yaml` (секреты).

### Использование

**Интерактивный режим:**

```bash
zev
```

Введите описание задачи, получите до 3 вариантов команд с пояснениями.

**Прямой запрос:**

```bash
zev "list all docker containers"
zev "find large files older than 30 days"
```

**История запросов:**

```bash
zev --recent
```

Откроет интерактивный селектор с последними 100 запросами.

## Провайдеры

### OpenAI

Поддерживаются как оригинальный OpenAI API, так и OpenAI-compatible роутеры (Azure OpenAI, LiteLLM, vLLM и др.).

**Переменные окружения:**

```bash
export OPENAI_API_KEY="sk-..."
export OPENAI_MODEL="gpt-4-turbo"
export OPENAI_BASE_URL="https://api.openai.com/v1"  # по умолчанию
```

**Пример для Azure OpenAI:**

```bash
export OPENAI_API_KEY="your-azure-key"
export OPENAI_MODEL="gpt-4"
export OPENAI_BASE_URL="https://your-resource.openai.azure.com/openai/deployments/your-deployment/chat/completions?api-version=2024-02-15-preview"
```

### Gemini

```bash
export GEMINI_API_KEY="your-key"
export GEMINI_MODEL="gemini-3-flash-preview"
```

### Ollama

Локальная модель без API ключа:

```bash
export OLLAMA_BASE_URL="http://localhost:11434/v1"
export OLLAMA_MODEL="llama3"
```

## Структура проекта

Проект следует архитектуре **vertical slices + ports and adapters**:

```
zev-go/
├── cmd/zev/              # Точка входа CLI
├── internal/
│   ├── core/
│   │   ├── config/       # Управление конфигурацией
│   │   └── domain/       # Доменные типы
│   └── entities/
│       ├── history/      # История запросов
│       │   └── service/
│       ├── query/        # Запросы к LLM
│       │   ├── service/
│       │   └── repository/
│       │       ├── factory/
│       │       ├── openai/
│       │       ├── gemini/
│       │       └── ollama/
│       └── setup/        # Первичная настройка
│           └── service/
└── pkg/
    └── tui/              # Terminal UI компоненты
        ├── prompt/       # Интерактивные промпты
        └── selector/     # Command selector
```

## Разработка

### Требования

- Go 1.23+
- golangci-lint

### Команды

**С использованием Make (рекомендуется):**

```bash
# Показать все доступные команды
make help

# Собрать
make build

# Собрать с версией
make build VERSION=1.0.0

# Запустить тесты
make test

# Запустить линтер
make lint

# Собрать и установить
make install

# Собрать для всех платформ
make build-all
```

**Напрямую через Go:**

```bash
# Сборка
go build -o zev ./cmd/zev

# С версией
go build -ldflags "-X main.version=1.0.0" -o zev ./cmd/zev

# Линтер
golangci-lint run --fix

# Тесты
go test ./...
```

## Лицензия

MIT

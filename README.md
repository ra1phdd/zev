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

- **OpenAI**: API key + опциональный custom base URL (для OpenAI-compatible провайдеров)
- **Gemini**: Google API key
- **Ollama**: локальный URL (по умолчанию `http://localhost:11434`)

При выборе OpenAI можно:
- Оставить base URL пустым → используется стандартный OpenAI API
- Указать custom URL → работа через совместимые провайдеры (Together, Anyscale, local proxies)

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

Поддерживаются как оригинальный OpenAI API, так и OpenAI-compatible провайдеры.

**Стандартный OpenAI:**

```bash
export OPENAI_API_KEY="sk-proj-..."
export OPENAI_MODEL="gpt-4-turbo"
export OPENAI_BASE_URL="https://api.openai.com/v1"  # по умолчанию
```

**OpenAI-compatible провайдеры:**

Together AI:
```bash
export OPENAI_API_KEY="your-together-key"
export OPENAI_MODEL="meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo"
export OPENAI_BASE_URL="https://api.together.xyz/v1"
```

Anyscale:
```bash
export OPENAI_API_KEY="your-anyscale-key"
export OPENAI_MODEL="meta-llama/Meta-Llama-3-70B-Instruct"
export OPENAI_BASE_URL="https://api.endpoints.anyscale.com/v1"
```

Local proxy:
```bash
export OPENAI_API_KEY="local"
export OPENAI_MODEL="gpt-4"
export OPENAI_BASE_URL="http://localhost:8000/v1"
```

### Gemini

```bash
export GEMINI_API_KEY="your-key"
export GEMINI_MODEL="gemini-3-flash-preview"
```

### Ollama

Локальная модель без API ключа:

```bash
export OLLAMA_BASE_URL="http://localhost:11434"  # по умолчанию
export OLLAMA_MODEL="llama2"
```

Или для удалённого Ollama:

```bash
export OLLAMA_BASE_URL="http://192.168.1.100:11434"
export OLLAMA_MODEL="mistral"
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

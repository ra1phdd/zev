<div align="center">

[English](../en/ARCHITECTURE.md) | **Русский**

</div>

# Архитектура

Zev — небольшая Go-утилита, построенная как **vertical slices с ports and adapters**.

```
cmd/zev/                        composition root (команды Cobra)
internal/core/
  domain/                       доменные типы, интерфейс InferenceProvider, ошибки
  config/                       загрузка ~/.zevrc (godotenv + caarlos0/env)
internal/entities/
  query/
    service/                    use case: сбор контекста, запрос провайдера, запись истории
    repository/openai/          OpenAI и OpenAI-совместимые провайдеры
    repository/gemini/          Google Gemini (raw HTTP + JSON response schema)
    repository/anthropic/       Anthropic Claude
    repository/ollama/          Ollama (переиспользует OpenAI-адаптер)
    repository/opencode/        OpenCode (SSE streaming, tool calls)
    repository/factory/         выбор провайдера по конфигу
    system_prompt.txt           системный промпт (go:embed)
    user_prompt.txt             шаблон промпта запроса (go:embed)
  history/service/              JSONL-история, 100 записей, авто-ротация
  alias/service/                алиасы команд с подстановкой аргументов
  script/service/               метаданные и файлы bash-скриптов
  setup/service/                интерактивный TUI-мастер настройки
pkg/tui/prompt/                 ввод текста и выбор (Bubble Tea)
pkg/tui/selector/               выбор подсказки с предупреждениями
pkg/tui/list/                  generic-рендерер списков
pkg/terminal/                   буфер обмена и помощники терминала
```

## Путь запроса

1. `cmd/zev` разбирает запрос (парсинг флагов на корневой команде отключён, чтобы
   алиасы с флагами работали корректно).
2. `query/service` собирает контекст окружения (ОС, шелл, cwd, установленные утилиты) и
   собирает промпт из `user_prompt.txt`.
3. `factory.NewProvider` создаёт реализацию `InferenceProvider` согласно `LLM_PROVIDER`.
4. Провайдер возвращает `OptionsResponse` с максимум тремя `Command`, у каждой — краткое
   пояснение и флаг `IsDangerous`.
5. Селектор отрисовывает варианты; выбранная команда подставляется в терминал или
   выполняется — в зависимости от `--execute` / `--inject` / `COMMAND_EXECUTION_MODE`.
6. Обмен записывается в `~/.zevhistory`.

## Ports и adapters

`domain.InferenceProvider` — единственный порт, от которого зависит сервис запросов:

```go
type InferenceProvider interface {
    GetOptions(ctx context.Context, prompt string, envContext string) (*OptionsResponse, error)
    GetOptionsWithHistory(ctx context.Context, history *ConversationHistory) (*OptionsResponse, error)
    Name() string
    Model() string
}
```

Интерфейсы объявляются на стороне потребителя; только `cmd/zev` знает о конкретных
адаптерах. Чтобы добавить провайдера, достаточно создать один пакет в
`internal/entities/query/repository/` и добавить один `case` в фабрику.

## Особенности провайдеров

- **OpenAI** — `github.com/sashabaranov/go-openai`, поддерживает кастомный base URL для
  любого OpenAI-совместимого роутера.
- **Gemini** — raw HTTP с JSON-схемой ответа.
- **Ollama** — переиспользует OpenAI-адаптер для локального эндпоинта.
- **OpenCode** — прямой HTTP-клиент: `stream: true`, функция-инструмент `read`,
  `stream_options.include_usage`, разбор SSE с накоплением `delta.content` и
  произвольные заголовки через `HTTP_HEADERS` (регистр ключей сохраняется).

## Как добавить провайдера

1. Создать `internal/entities/query/repository/<name>/`, реализующий
   `domain.InferenceProvider`.
2. Добавить поля конфигурации (ключ, модель, base URL) в `internal/core/config`.
3. Добавить `case` в `factory.NewProvider`.
4. Покрыть юнит-тестами на `httptest`.

## Тесты

Юнит-тесты лежат рядом с проверяемым кодом. Integration-тесты используют build tag
`integration` и запускаются отдельно. `make check` запускает линтер и весь набор тестов.

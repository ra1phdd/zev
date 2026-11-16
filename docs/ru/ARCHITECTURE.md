<div align="center">

[English](../en/ARCHITECTURE.md) | **Русский**

</div>

# Архитектура

Zev — небольшая Go-утилита, построенная как **vertical slices с ports and adapters**.

```
cmd/zev/                        команды Cobra, создание зависимостей и оркестрация
internal/core/
  domain/                       доменные типы, интерфейс InferenceProvider, ошибки
  config/                       загрузка ~/.zev/config (godotenv + caarlos0/env)
  httpclient/                   общий HTTP-клиент и транспорт
internal/entities/
  query/
    service/                    проверка запроса и контекст окружения
    repository/openai/          OpenAI и OpenAI-совместимые провайдеры
    repository/gemini/          Google Gemini (raw HTTP + JSON response schema)
    repository/anthropic/       Anthropic Claude
    repository/ollama/          Ollama (переиспользует OpenAI-адаптер)
    repository/opencode/        OpenCode (SSE streaming, tool calls)
    repository/factory/         выбор провайдера по конфигу
    repository/prompt.go          встраивает system_prompt.txt; шаблон пользователя — "%s"
  history/service/              JSONL-история, 100 записей, авто-ротация
  alias/{service,repository}/   подстановка аргументов и YAML-хранилище
  script/{service,repository}/  bash-скрипты, метаданные, редактор и выполнение
  setup/service/                интерактивный TUI-мастер настройки
pkg/tui/prompt/                 ввод текста с переносами
pkg/tui/list/                   списки и предупреждения для команд
pkg/terminal/                   shell и платформенная вставка в терминал
```

## Путь запроса

1. `cmd/zev/query.go` вручную разбирает флаги корневой команды и загружает конфигурацию.
2. Сначала ищется алиас, затем сохранённый скрипт. Совпадение выполняется напрямую;
   только оставшийся ввод передаётся LLM.
3. CLI создаёт HTTP-клиент и вызывает `factory.NewProvider`. Сервис запроса собирает
   ОС, архитектуру, shell, user, home и cwd; CLI формирует системный промпт из
   встроенного `system_prompt.txt` с этим контекстом окружения, а исходный запрос
   оставляет сообщением пользователя (`UserPromptTemplate = "%s"`). Установленные
   утилиты не сканируются.
4. CLI вызывает `InferenceProvider.GetOptionsWithHistory`. Провайдеры декодируют
   `OptionsResponse`; сервис запроса проверяет ответ до выбора и отклоняет больше
   трёх вариантов, а также превышение лимитов полей и кодированного ответа.
5. `inject` показывает варианты и позволяет уточнять запрос в активном диалоге,
   который хранится в памяти. Выбранная команда вставляется в терминал либо печатается,
   если вставка не поддерживается или запрещена.
6. `execute` выполняет первый вариант без выбора и подтверждения. Выбранная команда
   повторно проверяется перед доставкой; предупреждение печатается после выполнения,
   а не используется как защитный барьер.
7. Если история включена, CLI сохраняет ответ и выбранную команду в
   `~/.zev/history`.

Оркестрация диалога, выбор, выполнение и запись истории сейчас находятся в CLI,
а не в `query/service`. Корневой отменяемый контекст `signal.NotifyContext` передаётся
в вызовы провайдера/HTTP, повторы, TUI и выполнение команд/редактора.
`CONTEXT_ENABLED=false` / `--no-context` отключает автоматически добавляемый контекст
окружения, но не текст запроса. Активный диалог остаётся в памяти даже при
`HISTORY_ENABLED=false` / `--no-history`; в JSONL-историю записывается только результат
запроса, если история включена.

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

Фабрика импортирует конкретные адаптеры провайдеров. Сервисы алиасов и скриптов
пока используют конкретные репозитории.
История работает с файлами внутри сервиса, без отдельного адаптера репозитория.

## Особенности провайдеров

- **OpenAI** — `github.com/sashabaranov/go-openai`, поддерживает кастомный base URL для
  любого OpenAI-совместимого роутера.
- **Gemini** — raw HTTP с JSON-схемой ответа.
- **Anthropic** — `github.com/liushuangls/go-anthropic/v2`.
- **Ollama** — переиспользует OpenAI-адаптер для локального эндпоинта.
- **OpenCode** — прямой HTTP-клиент: `stream: true`, функция-инструмент `read`,
  `stream_options.include_usage`, разбор SSE с накоплением `delta.content` и
  произвольные заголовки через `HTTP_HEADERS` (регистр ключей сохраняется).

## Как добавить провайдера

1. Создать `internal/entities/query/repository/<name>/`, реализующий
   `domain.InferenceProvider`.
2. Добавить поля конфигурации (ключ, модель, base URL) в `internal/core/config`.
3. Добавить `case` в `factory.NewProvider`, обновить setup/выбор провайдера и обе
   версии документации конфигурации.
4. Покрыть юнит-тестами на `httptest`.

## Тесты

Тесты лежат рядом с кодом. `make check` запускает проверки форматирования, линтер,
отчёт deadcode приложения, vet и обычные unit-тесты; `make test-race` — отдельно.
Цели tagged-тестов существуют, но в отслеживаемых тестах пока нет наборов
`integration`, `e2e` или `manual`. Команды описаны в [DEVELOPMENT.md](DEVELOPMENT.md),
эксплуатационные ограничения — в [PROJECT.md](../../PROJECT.md).

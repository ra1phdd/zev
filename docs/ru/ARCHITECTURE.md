<div align="center">

[English](../en/ARCHITECTURE.md) | **Русский**

</div>

# Архитектура

Zev — небольшая Go-утилита, построенная как **vertical slices с ports and adapters**.

```
cmd/zev/                        тонкий launcher процесса
internal/cli/                    команды Cobra, создание зависимостей и оркестрация
internal/core/
  domain/                       доменные типы, интерфейс InferenceProvider, ошибки
  config/                       загрузка ~/.zev/config.yaml (только текущий YAML)
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
  history/service/              жизненный цикл, consumer ports, lexical/vector-оркестрация
  history/repository/           текущие SQLite-транзакции и производные индексы
  history/client/               opt-in OpenAI-совместимый embedding HTTP-адаптер
  history/lexical/              локальная нормализация, Snowball stemming и typo-примитивы
  alias/{service,repository}/   подстановка аргументов и YAML-хранилище
  script/{service,repository}/  bash-скрипты, метаданные, редактор и выполнение
  setup/service/                интерактивный TUI-мастер настройки
internal/migrations/             упорядоченные миграторы хранилищ и registry
  filetxn/                       общие filesystem staging, publication и journal recovery для миграций
  history/                       импорт JSONL из v1.0.0 и встроенная SQL-миграция SQLite
  metadatafile/                  migration конфигурации/alias/script metadata
  config/                        migration dotenv v1.0.0
  aliases/, scripts/              шаги миграции выпущенных metadata
pkg/tui/prompt/                 ввод текста с переносами
pkg/tui/list/                   списки и предупреждения для команд
pkg/terminal/                   shell и платформенная вставка в терминал
```

## Граница миграции

Оркестрация миграций выполняется до открытия выбранного хранилища. Поддерживаемые
legacy-входы — dotenv-конфигурация v1.0.0, неверсированные metadata алиасов/скриптов
v1.0.0 и JSONL-история v1.0.0. Текущие readers в `internal/core` и
`internal/entities` принимают только актуальные форматы; они не распознают legacy-данные
и не вызывают миграцию при чтении или записи. Development-снимки не являются входом
совместимости. Будущие шаги сохраняются только для переходов между выпущенными форматами.

JSONL-importer истории отделён от одной встроенной SQLite-базы `.up.sql`. Сначала он
импортирует старые записи, затем текущий repository открывает актуальный SQLite-формат.
База не воспроизводит историю dev-схем.

## Путь запроса

1. `internal/cli/query.go` вручную разбирает флаги корневой команды и загружает конфигурацию.
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
7. Если история включена, CLI открывает `~/.zev/history.db` и передаёт хранилище
   сервису жизненного цикла истории. Начальный ввод сохраняется до вызова провайдера;
   ответы, выбор команды, результаты выполнения и итоговый статус — по мере работы.
   Каждое уточнение записывается отдельно до обращения к провайдеру. Ошибки истории
   дают предупреждения, не блокируя запрос к провайдеру команд.

Оркестрация диалога, выбор и выполнение находятся в CLI, не в `query/service`;
постоянная запись делегируется `history/service`. Корневой отменяемый контекст
`signal.NotifyContext` передаётся
в вызовы провайдера/HTTP, повторы, TUI и выполнение команд/редактора.
`CONTEXT_ENABLED=false` / `--no-context` отключает автоматически добавляемый контекст
окружения, но не текст запроса. Активный диалог остаётся в памяти даже при
`HISTORY_ENABLED=false` / `--no-history`; эти настройки запрещают открытие хранилища
истории, а значит, миграцию и embeddings для запроса. Постоянная история не становится
контекстом нового запроса к модели автоматически.

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
пока используют конкретные репозитории. Порты хранения и embeddings истории объявлены
на стороне потребителя/сервиса. Composition root открывает и закрывает repository;
CLI-обработчики не составляют SQL.

## Границы и ограничения истории

- `history/repository` отвечает за текущее SQLite-хранилище, транзакции связанных
  записей, внешние ключи, WAL и производные индексы. Его открытие не импортирует legacy
  JSONL и не разбирает форматы миграции.
- `internal/migrations/filetxn` предоставляет общие filesystem staging, atomic
  publication, классификацию сохранности и recovery для распознанных journal текущего
  протокола. Он не реализует SQLite-транзакции, rollback базы данных или rollback CRUD
  алиасов/скриптов. Миграции не создают архивы или backup-копии; существующие
  пользовательские backup и другие копии вне его scope. Артефакты dev-снимков без
  маркера или публикации не восстанавливаются и не обходятся.
- `internal/migrations/history` отвечает за поддерживаемый JSONL v1.0.0,
  проверку снимка, журнал импорта и удаление/восстановление источника. CLI-оркестрация
  проверяет его до открытия текущего store. Этот путь используют query с историей и
  явные команды истории, но не help, version, локальный doctor или query с отключённой
  историей. Миграция не использует сеть.
- `history/service` отвечает за жизненный цикл диалога и оркестрацию поиска. Полные ответы
  и варианты команд записываются атомарно; выбор не означает выполнение, а завершение
  взаимодействия не доказывает запуск вставленного ввода. Ошибка записи истории даёт
  предупреждение, но не должна повторять уже выполненную команду.
- `history/lexical` применяет одинаковую версионируемую нормализацию и русский/английский
  Snowball-профиль к индексу и запросам. Это ограниченный stemming, не лемматизация
  нерегулярных форм. Repository ставит surface-совпадения выше morphology и typo-only,
  используя RRF внутри уровней. Service отдельно объединяет lexical/vector-списки
  через RRF; hybrid-порядок не сохраняет эти lexical tiers.
- `history/client` — отдельный opt-in embedding-адаптер, не провайдер команд. Допустимы
  только начальные user-сообщения и уточнения. Команды, assistant и system text не
  векторизуются. Совместимость задают модель, размерность, профиль и content hash.

Semantic search по умолчанию отключён, не индексирует старую историю автоматически и не
подставляет найденное в новые промпты. Полный cosine scan имеет сложность O(N × dimensions):
это не ANN и не обещание постоянной задержки. Literal-подстрока может требовать scan
канонического текста. Удаление логическое, не forensic erase WAL, backup и удалённых копий.

CLI загружает конфигурацию истории и передаёт opt-in embedding-клиент в search service.
`history index status` показывает покрытие; `history index rebuild --fts` перестраивает
локальные индексы, а `--vectors` выводит provider/model/eligible count и требует явный
`--yes` для согласия на передачу. Обычный query не создаёт векторы.
`history search --cursor` продолжает страницы, привязанные к запросу/профилю/ревизии.
Фильтры документов предшествуют FTS top-K; ограниченные lexical/typo-списки и hybrid-
окна отражают обнаруженное усечение, а не гарантируют неограниченный recall.
Semantic/hybrid явно переходят на lexical при отключённых embeddings или ошибке
векторизации поискового запроса. Точные лимиты, границы восстановления и приватности —
в [Истории](HISTORY.md). Кросс-компиляция не проверяет native Windows runtime или ACL;
это описание не означает прохождения всех release gates.

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

Unit-тесты лежат рядом с кодом, включая CLI-тесты в `internal/cli`.
Сквозные сценарии находятся в `e2e/`, собирают реальный бинарник и запускаются через
`make test-e2e`. `make check` проверяет форматирование, lint, deadcode, vet и unit-тесты;
`make test-race` запускается отдельно. Команды описаны в [DEVELOPMENT.md](DEVELOPMENT.md),
эксплуатационные ограничения — в [PROJECT.md](../../PROJECT.md).

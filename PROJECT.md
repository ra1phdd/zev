# Zev — профиль проекта

Подтверждённые сведения о репозитории; общие правила работы находятся в
[AGENTS.md](AGENTS.md). Этот профиль описывает фактический код, а не целевую архитектуру.

SQLite history — Unreleased. Query lifecycle, автоматическая JSONL-миграция, CLI-поиск
с opt-in embeddings, `history index` и `--cursor` подключены. Ниже отражены фактические
ограничения; наличие исходников или отдельных зелёных проверок не означает прохождения
всех release gates или native-проверки Windows.

## Назначение и границы

- **Zev** — локальная CLI-утилита, которая превращает запросы на естественном языке
  в варианты терминальных команд с пояснениями через выбранного LLM-провайдера.
- Дополнительные возможности: интерактивное уточнение запроса, просмотр последних
  запросов, шаблонные алиасы, сохранённые bash-скрипты и мастер настройки.
- Go-модуль: `github.com/ra1phdd/zev`; минимальная версия из [go.mod](go.mod) —
  **Go 1.27.2**. Лицензия — [MIT](LICENSE).
- Текущая релизная версия в [VERSION](VERSION) и существующем теге — **1.0.0**.
  Изменения после `v1.0.0` относятся к `Unreleased`, а не к новому релизу.
- Это клиентское приложение, не сервер: в отслеживаемом коде нет собственного
  HTTP/gRPC API, health endpoint, метрик или tracing. История использует локальную SQLite
  БД и schema migrations; отдельный OpenAI-compatible HTTP endpoint нужен только для
  явно включённых embeddings.
  Качество и доступность ответов зависят от настроенной модели и её API.

## Архитектура и путь выполнения

Точка входа — [cmd/zev/main.go](cmd/zev/main.go). Здесь регистрируются Cobra-команды
`query`, `history`, `setup`, `alias`, `script`, `version`; корневая команда также
обрабатывает запрос. Файлы `cmd/zev/*.go` создают зависимости и адаптируют CLI/TUI.

Основные пакеты:

| Путь | Фактическая ответственность |
| --- | --- |
| `internal/core/domain` | `Command`, `OptionsResponse`, `Alias`, `Script`, сообщения диалога, ошибки и порт `InferenceProvider` |
| `internal/core/config` | Defaults, загрузка окружения и `~/.zev/config`, сохранение конфигурации |
| `internal/core/httpclient` | Общий HTTP-клиент, заголовки, ограниченные повторы и безопасная диагностика |
| `internal/core/filestore` | Атомарная замена файлов, безопасное чтение, кооперативные межпроцессные блокировки |
| `internal/core/childprocess` | Управление дочерними процессами, отмена и очистка принадлежащих группе/job потомков |
| `internal/entities/query/service` | Проверка запроса в `GetOptions`, сбор контекста ОС/архитектуры/shell/user/home/cwd |
| `internal/entities/query/repository` | Общие промпты, извлечение JSON, преобразование сетевых ошибок |
| `internal/entities/query/repository/{openai,gemini,anthropic,ollama,opencode}` | Адаптеры пяти LLM-провайдеров |
| `internal/entities/query/repository/factory` | Выбор адаптера по `LLM_PROVIDER` |
| `internal/entities/history/service` | Жизненный цикл диалога, consumer-side storage/embedding ports, оркестрация поиска |
| `internal/entities/history/repository` | SQLite, версии схемы, транзакции, FTS-документы, пагинация и удаление |
| `internal/entities/history/lexical` | Версионируемая нормализация, русский/английский stemmer, bounded typo primitives |
| `internal/entities/history/client` | Opt-in OpenAI-compatible embedding-клиент только для user-сообщений |
| `internal/entities/alias/{service,repository}` | Подстановка аргументов и YAML-хранилище алиасов |
| `internal/entities/script/{service,repository}` | Метаданные, файлы, редактор и выполнение bash-скриптов |
| `internal/entities/setup/service` | Интерактивный выбор провайдера и сохранение настроек |
| `pkg/tui/list`, `pkg/tui/prompt` | Списки выбора, предупреждения и ввод с переносом строк |
| `pkg/terminal` | Выполнение через shell и платформенная вставка команды в терминал |

Реальный путь запроса в [cmd/zev/query.go](cmd/zev/query.go):

1. Ручной разбор флагов сохраняет возможность передавать флаги алиасам.
2. Загрузка конфигурации; поиск алиаса, затем скрипта по первому слову запроса.
   Найденные алиас или скрипт выполняются напрямую, без обращения к LLM.
3. Если совпадений нет, создаются HTTP-клиент и провайдер. При включённой истории
   composition root открывает SQLite store и lifecycle записывает первоначальный user message
   до вызова провайдера; сервис запроса собирает контекст окружения, CLI формирует первое
   сообщение и вызывает `InferenceProvider.GetOptionsWithHistory`.
4. В режиме `inject` CLI показывает варианты и позволяет уточнять запрос с
   сохранением диалога в памяти. В режиме `execute` сразу выбирается первый вариант.
5. Команда вставляется в терминал либо выполняется; выбор и результат сохраняются в
   текущем SQLite-диалоге. Ответы и варианты команд сохраняются отдельными записями;
   каждое уточнение — отдельным user message до следующего вызова провайдера. Сбой
   открытия/сохранения истории выводится как предупреждение, не останавливая query.

Структура следует vertical slices + ports/adapters. Оркестрация диалога, выбор и
выполнение находятся в CLI, не в `query/service`; CLI делегирует постоянную историю
`history/service`, который получает persistence port от composition root.
Системный промпт встроен через `go:embed` из
`internal/entities/query/repository/system_prompt.txt`; пользовательское сообщение —
исходный запрос (`UserPromptTemplate = "%s"`), отдельный файл не встраивается.
Ответы всех пяти провайдеров и существующая история валидируются до выбора команды;
перед доставкой выбранная подсказка проверяется повторно. Лимиты: 3 варианта,
4096 байт UTF-8 на команду, 2048 байт на пояснение, 32 КиБ структурированного JSON
и 256 КиБ тела HTTP-ответа провайдера (включая SSE OpenCode). Это фиксированные
лимиты, не конфигурация. В ответе разрешены tab/LF/CR, но вставка в терминал
отклоняет все управляющие символы до записи первого байта.

## Зависимости и интеграции

Версии фиксируются в `go.mod`/`go.sum`:

- Cobra `v1.10.2` — CLI; Bubble Tea `v1.3.10`, Bubbles `v1.0.0`, Lipgloss `v1.1.0` — TUI.
- `go-openai v1.43.0` — OpenAI и совместимые API; Ollama переиспользует этот адаптер.
- `go-anthropic/v2 v2.25.0` — Anthropic; Gemini использует HTTP/JSON,
  OpenCode — HTTP/SSE с накоплением потокового ответа.
- `godotenv v1.5.1` и `caarlos0/env/v11 v11.4.1` — конфигурация;
  `yaml.v3 v3.0.1` — алиасы и метаданные скриптов; `x/term` — терминал.
- `modernc.org/sqlite v1.62.0` — SQLite-драйвер истории; `kljensen/snowball v0.10.0`
  и `x/text v0.43.0` — локальный русско-английский lexical-профиль.
  `CGO_ENABLED=0 make build-all` проверяет кросс-компиляцию четырёх Make-целей,
  но не native-поведение целевых ОС или полную матрицу GoReleaser.

Для реального запроса нужны доступный endpoint и настройки выбранного провайдера;
для Ollama — работающий локальный/удалённый совместимый endpoint.
Обычные unit-тесты не требуют провайдерских ключей или внешней LLM.

## Конфигурация и данные

Источник истины — [internal/core/config/config.go](internal/core/config/config.go).
Приоритет значений: **существующее окружение → `~/.zev/config` → defaults**.
Файл имеет dotenv-формат `KEY=VALUE`, а не YAML/JSON. Его создаёт `zev setup`;
`zev setup mode` и `zev setup headers` меняют соответствующие настройки.

- `LLM_PROVIDER`: `openai` (default), `gemini`, `anthropic`, `ollama`, `opencode`.
- `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` используются также OpenCode.
- Для Gemini и Anthropic предусмотрены собственные `*_API_KEY`, `*_MODEL`, `*_BASE_URL`;
  для Ollama — `OLLAMA_MODEL`, `OLLAMA_BASE_URL`.
- `HTTP_HEADERS` — пары `Key:Value` через запятую; разбиение значения — по первому `:`.
- `COMMAND_EXECUTION_MODE` по умолчанию `inject`; CLI-флаги переопределяют настройку.
- `SCRIPT_EDITOR`, затем `EDITOR`, затем доступный `nano`/`vim`/`vi` определяют редактор;
  `SKIP_EDITOR=1` отключает его запуск. Редактор работает с приватной временной
  копией; результат атомарно заменяет скрипт после проверки исходной идентичности.
  Устаревшее редактирование не перезаписывает новую версию другого кооперативного
  редактора. Ошибка/отмена редактора оставляет уже зарегистрированный скрипт,
  временная копия очищается.
- `HISTORY_ENABLED` и `CONTEXT_ENABLED` по умолчанию `true`; флаги `--no-history`
  и `--no-context` отключают соответствующее поведение для одного запроса.
  `HISTORY_SEMANTIC_ENABLED` по умолчанию `false`; при включении требуется отдельный
  `openai-compatible` provider, base URL, model, positive dimensions, API key (если нужен)
  и `HISTORY_EMBEDDING_MAX_BYTES` (default `32768`). Отключение истории не удаляет записи,
  не открывает SQLite и не отключает диалог в памяти.

| Путь | Формат и назначение |
| --- | --- |
| `~/.zev/config` | Конфигурация и API-ключи в открытом виде; при создании режим `0600` |
| `~/.zev/aliases` | YAML-отображение имени алиаса в шаблон команды |
| `~/.zev/scripts` | YAML-метаданные: имя, файл и описание скрипта |
| `~/.zev/script_files/*.sh` | Тела bash-скриптов; при создании режим `0700` |
| `~/.zev/history.db` | SQLite с постоянными диалогами, сообщениями, ответами, командами и индексами; без лимита количества |
| `~/.zev/history` | Legacy JSONL: автоматически импортируется/архивируется при открытии repository из query или команд истории |
| `~/.zev/history.migrated.*.jsonl` | Архив исходного JSONL для ручного восстановления/audit |

Query recorder сохраняет первоначальный запрос, каждое уточнение, ответы, варианты
команд, выбор и наблюдаемый результат. Выбор/вставка не означают выполнение; аварийный
выход может оставить `started` и неизвестный outcome. При открытии repository автоматически
импортирует legacy JSONL под lock: проверяет fingerprint и количество, архивирует в
`history.migrated.<sha256-prefix>.<UTC-timestamp>.<UUID>.jsonl`, затем завершает ledger.
Миграция не использует сеть; невалидная строка блокирует активацию, ошибки не печатают
payload. Этот путь используют и query с историей, и explicit history commands.
Конфигурация и алиасы используют атомарную замену; SQLite — WAL-транзакции.
Семантика выключена по умолчанию; при включении CLI подключает отдельно настроенный
embedding-клиент к semantic/hybrid search и явному vector rebuild. Отключённые embeddings
или ошибка векторизации query дают видимый lexical fallback; пустое покрытие не создаёт
векторы автоматически. Для embeddings допустимы только user initial/refinement, не
команды, assistant/system text или metadata. Query не создаёт векторы, не делает backfill
и не использует историю как RAG. Snowball даёт ограниченный stemming, не связывает
go/went/gone. Lexical сохраняет quality tiers и all-term matching; typo-only ранжируется
по проверенной близости с преобразованием в RRF, hybrid отдельно объединяет списки.
Фильтры документов применяются до FTS top-K: 5000 документов на FTS-список, 256 typo-
кандидатов на lookup/expansion path, 1000 результатов каждого hybrid-списка. Semantic
сканирует актуальные совместимые векторы за O(N × dimensions), удерживая страницу
с lookahead. `truncated` отражает обнаруженные ограничения, не гарантирует полный
recall; `nextCursor` передаётся через `--cursor` с тем же запросом/лимитом/фильтрами.
Изменения данных/индексов инвалидируют курсор. Remote embeddings раскрывают user text;
localhost может быть forwarding proxy. `history index status --json` показывает покрытие;
`history index rebuild` требует ровно один из `--fts` или `--vectors`. Vector rebuild
показывает provider/model/общее eligible count и требует явный `--yes`; failed повторяются
только с `--retry-failed`. Локальный FTS rebuild не отправляет embedding-запросов.
Удаление логическое, не гарантирует forensic erase WAL/free pages, backup или JSONL-архива.
Каталоги создаются с `0700`, файлы данных — `0600` на Unix. Постоянные `*.lock`
нельзя удалять или заменять: они задают идентичность межпроцессного lock.
Повреждённые данные вызывают ошибку, а не тихую перезапись пустым хранилищем.

Symlink вместо `~/.zev` или файла данных отклоняется; доверенные родительские
алиасы допустимы. Скрипты дополнительно ограничены относительными путями внутри
`script_files`, без traversal, symlink в компонентах приложения или нескольких
жёстких ссылок на файл; выполнение использует уже открытый проверенный файл.
Протокол не изолирует от враждебных same-UID процессов и писателей в обход lock.

Ошибка атомарной записи до rename оставляет предыдущий файл неизменным.
После rename ошибка fsync каталога возвращает `ErrCommitted`: новая версия уже
видна, но сохранность после аварии неизвестна. Для классификации используется
`errors.Is` с `filestore.ErrCommitted` либо `ErrCommitted` репозитория скриптов;
повтор неидемпотентной файловой операции без проверки может повторить её эффект.
Ошибки очистки тоже могут сопровождать выполненное изменение. Создание/удаление
скрипта сериализует метаданные и тело, с попыткой отката до фиксации, но не даёт
crash-atomic транзакции над двумя файлами. На Windows directory-fsync не выполняется;
Unix mode bits не являются доказательством настроенных Windows ACL.

## Разработка и проверки

Команды определены в [Makefile](Makefile). Для проверок нужны Go и утилиты
`gofmt`, `gofumpt`, `goimports`, `golangci-lint` v2, `deadcode` в `PATH`.

```bash
make help
make build
make run ARGS=version
make setup ARGS=--help
make test
make test-race
make lint
make check
make build-all
make release-check
make clean
```

- `make test` запускает `go vet ./...` и `go test ./...`; race-проверка — отдельная цель.
  Тесты lifecycle, migration/archive, транзакций, lexical/vector search, CLI и embedding
  client находятся рядом с кодом; их наличие не заменяет запуск или release gates.
- `make lint` сначала проверяет форматирование, затем запускает
  `golangci-lint run --fix` и `deadcode -test $(APP)`. Линтер может менять исходники;
  отчёт deadcode требует анализа и сам по себе не разрешает удаление функций.
- `make fmt` применяет gofmt, goimports и gofumpt ко всем найденным Go-файлам;
  это отдельная изменяющая исходники операция, не обязательный шаг перед каждым запуском.
- `GO`, `APP` (default `./cmd/zev`), `BIN` (default `bin/zev`), `FLAGS`, `VERSION` (default `dev`)
  переопределяются. `BIN` проверяется для сборки и очистки: файл внутри `bin/`, без `..`.
- `make build-all` собирает Linux amd64, macOS amd64/arm64 и Windows amd64.
  Кросс-компиляция не проверяет поведение на целевой ОС.
- Сборка использует `-s -w -X main.version=$(VERSION) -buildid=` и `-trimpath`.
  Удаляются build ID и локальные пути; размер уменьшается за счёт
  удаления таблицы символов и DWARF, не за счёт дополнительных оптимизаций CPU.
  `make build FLAGS=` оставляет отладочную информацию и значение версии `dev` из кода.
- `make clean` удаляет только `BIN` и его четыре платформенных варианта, не весь `bin/`.
- `make install` вызывает `go install` с учётом `GOBIN`/`GOPATH/bin`, без лишней локальной сборки.
- `make dev` использует `-vv`; `make watch` требует `entr` и запускает `make run`.

Unit-тесты находятся рядом с кодом в `*_test.go`, используют стандартный `testing`
и локальные заглушки/`httptest`. В отслеживаемых тестах текущей истории нет наборов
с тегами `integration`, `e2e`, `manual`; цели `make test-integration` и `make test-e2e`
зарезервированы и включают обычные unit-тесты. `make test-manual ALLOW_MANUAL_TESTS=1`
требует отдельного разрешения и запрещён при заданном `CI`.
Существующего target для генерации исходников нет.

## Сборка и выпуск

[.goreleaser.yaml](.goreleaser.yaml) описывает релизные сборки без CGO:
Linux amd64/arm64/armv7, macOS amd64/arm64, Windows amd64/arm64; архивы `tar.gz`,
для Windows — `zip`, плюс `checksums.txt`. Версия внедряется в `main.version`;
GoReleaser уже использует `-s -w` и `-trimpath`.

[GitHub workflow](.github/workflows/release.yml) запускается при изменении `VERSION`
в `main` или вручную и устанавливает закреплённые версии релизных инструментов.
Он и `make release` используют [deploy/release.sh](deploy/release.sh):
`make fmt`, `make lint`, `make test`, `make test-race`, `make build VERSION=<version>`
выполняются до создания или push тега. Изменения отслеживаемых файлов относительно
`HEAD` после проверок и hooks сборки останавливают выпуск. Существующий тег должен
указывать на `HEAD`; принудительной замены нет. Затем GoReleaser собирает с `--skip=publish`,
[deploy/publish-release.sh](deploy/publish-release.sh) проверяет архивы по checksums,
и только после этого выполняется push тега. Публикация сначала создаёт черновик
с checksums, затем загружает архивы и делает релиз публичным. Сбой публикации может
оставить отправленный тег или черновик; перед повтором проверяют удалённое состояние.
`make release-check` только валидирует конфигурацию. `make release` публикует релиз;
без явного разрешения его не запускают.
`make release-snapshot` не публикует релиз, но выполняет hooks (включая `go mod tidy`)
и очищает `dist/`; это не read-only проверка. Вывод GoReleaser пока идёт в `dist/`,
в отличие от локальных Make-сборок в `bin/`.

В [deploy/](deploy/) находятся установщики и деинсталляторы для Unix и PowerShell;
установщики скачивают релизный архив и требуют проверки SHA-256. Недоступные
`checksums.txt`/hash-утилиты, некорректная или неоднозначная запись и несовпадение
хеша останавливают установку до замены бинарника. Проверка целостности по checksums
из того же релиза не является независимой подписью издателя. В отслеживаемом
дереве нет Dockerfile или контейнерного способа запуска приложения.

## Эксплуатационные ограничения

- `inject` использует TIOCSTI на Linux/macOS и требует TTY и разрешения ОС.
  При отказе или на Windows CLI печатает команду вместо вставки.
- `execute` выполняет **первую подсказку без подтверждения**. Предупреждение
  `IsDangerous` приходит от LLM и в этой ветке печатается после выполнения:
  это не гарантия безопасности. Алиасы выполняются через `$SHELL -c` (fallback `sh`),
  скрипты — через `bash`; на Windows для этих путей нужен доступный shell/bash.
- По умолчанию провайдеру передаются имя пользователя, домашний каталог и cwd
  вместе с контекстом ОС; `CONTEXT_ENABLED=false`/`--no-context` отключают эти
  автоматически добавляемые сведения, но не текст запроса. История хранит запросы
  и команды, если не отключена. API-ключи и значения заголовков вводятся скрыто
  и маскируются в setup. Диагностика провайдеров не печатает raw payload, ключи
  или URL; команды и пользовательский текст всё равно могут попадать в обычный
  вывод и включённую историю. Не вводите секреты в запросы.
- Наблюдаемость — `log/slog` в stderr: `-v` включает Info, `-vv` — Debug,
  `-vvv` добавляет source location; `LOG_LEVEL` не является настройкой CLI.
- Корневой CLI использует `signal.NotifyContext` для Ctrl-C/SIGTERM и передаёт
  контекст в LLM/HTTP, ожидание повторов, TUI и запуск команд/редактора.
  Дочерние процессы используют Unix process group или Windows kill-on-close job
  (создание suspended → assignment → resume). Оставшиеся управляемые потомки
  завершаются и при отмене, и после обычного выхода ведущего процесса, до его reap.
  Отделившиеся Unix-группы/сессии и внешние брокеры находятся вне управления;
  произвольный блокирующий I/O вызывающей стороны не ограничен очисткой pipe.
  Это не sandbox. Windows-реализация требует native runtime-проверки;
  кросс-компиляция её не доказывает. Некоторые файловые API без контекста
  сохраняют ожидание lock без deadline.
- История использует SQLite WAL и append/update-транзакции без count-based rotation.
  Disabled query history не открывает БД. CLI semantic composition, index maintenance
  и cursor input доступны; детали — в [Истории](docs/ru/HISTORY.md). Полный cosine scan
  имеет сложность O(N × dimensions), постоянная задержка не гарантирована.
- Тексты CLI/TUI находятся в исходниках; пакета `internal/core/i18n` нет.

Это описание существующих ограничений, а не разрешение ослаблять проверки или
расширять scope изменений. Изменение исполнения команд, контекстов или файловых
контрактов требует отдельной реализации и соответствующих тестов.

## Контракты и документация

- [README.ru.md](README.ru.md) / [README.md](README.md) — использование и установка.
- [Конфигурация](docs/ru/CONFIGURATION.md) — параметры и файлы данных.
- [История](docs/ru/HISTORY.md) — постоянное хранилище, миграция, поиск и semantic opt-in.
- [Архитектура](docs/ru/ARCHITECTURE.md) — пакеты и порт провайдера.
- [Разработка](docs/ru/DEVELOPMENT.md) и [участие](docs/ru/CONTRIBUTING.md) — команды и правила.
- [CHANGELOG.md](CHANGELOG.md) — пользовательские изменения по существующим релизам.
- [DELICIOUS.md](DELICIOUS.md) — индекс ADR; сейчас список решений пуст.
- Go-контракт провайдера — [domain/provider.go](internal/core/domain/provider.go),
  JSON ответа — [domain/command.go](internal/core/domain/command.go).
- Контракт истории — [history/service/types.go](internal/entities/history/service/types.go),
  SQLite — [history/repository/store.go](internal/entities/history/repository/store.go),
  lexical — [history/lexical/lexical.go](internal/entities/history/lexical/lexical.go),
  embedding client — [history/client/client.go](internal/entities/history/client/client.go).

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

## Правила миграций данных

- **Поддерживаем форматы выпущенных релизов, а не историю коммитов.** Источник для шага проверяем по релизному тегу и закрепляем тестовым примером данных этого релиза. До первой публикации схемы её можно менять на месте. Для промежуточных dev-сборок не добавляем версии, миграции или fallback-декодеры. После публикации шаг сохраняем неизменным: пользователь может пропустить несколько релизов.
- **`internal/core` и `internal/entities` работают только с актуальным форматом.** Они не распознают старые файлы, не декодируют legacy и не вызывают миграторы при чтении или записи. До открытия нужного хранилища CLI явно вызывает `internal/migrations/<формат>`. Мигратор может пользоваться актуальным кодеком для проверки результата; обратной зависимости быть не должно. Неизвестный формат не перезаписываем.
- **История:** в `v1.0.0` был только JSONL (`~/.zev/history`). Для перехода нужны один импортёр этого формата и одна базовая SQL-миграция, создающая полную первую схему SQLite. Промежуточные SQLite-схемы разработки в цепочку не входят. SQL храним в `.sql` и встраиваем в бинарник; архивных копий исходных данных не создаём.

## Архитектура и путь выполнения

Точка входа — [cmd/zev/main.go](cmd/zev/main.go), тонкий launcher. Cobra-команды
и orchestration находятся в `internal/cli`; сквозные сценарии — в `e2e/`.

Основные пакеты:

| Путь | Фактическая ответственность |
| --- | --- |
| `internal/core/domain` | `Command`, `OptionsResponse`, `Alias`, `Script`, сообщения диалога, ошибки и порт `InferenceProvider` |
| `internal/core/config` | Defaults, загрузка и сохранение `~/.zev/config.yaml`, выбор профиля провайдера |
| `internal/migrations` | Реестр обновления хранилищ; каталоги `config`, `aliases`, `scripts`, `history`. В `<формат>.vN.go` N — целевая версия, а не версия Zev. Миграции не создают архивы или резервные копии. См. [UPDATE](docs/ru/UPDATE.md). |
| `internal/migrations/filetxn` | Общие staging, atomic publication, классификация сохранности и journal recovery для файловых миграций; не транзакции SQLite и не rollback CRUD скриптов. |
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
| `internal/entities/update/{service,client,repository}` | Выбор стабильного GitHub Release, проверка SHA-256, замена бинарника и запуск миграций новой версией |
| `pkg/tui/list`, `pkg/tui/prompt` | Списки выбора, предупреждения и ввод с переносом строк |
| `pkg/terminal` | Выполнение через shell и платформенная вставка команды в терминал |

Общий HTTP-клиент сохраняет заголовки на время вызова, но redirects не следует:
redirect response возвращается вызывающему коду, а cross-origin redirect отклоняется до
контакта с новым origin. Это сохраняет привязку интерактивного consent к раскрытым URL и body.

Реальный путь запроса в [internal/cli/query.go](internal/cli/query.go):

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

Корневой dispatch разбирает исходный `argv` до соединения слов в prompt. Built-in-команды
имеют приоритет над alias/script; корневые flags и `--help`, стоящие до имени, не передают
управление entity, а `name -- ...` передаёт хвост буквально. Явные `zev query` и
`zev continue` сохраняют query/provider path: `zev query --print <alias>` выполняет
ровно один named machine lookup и печатает результат без выполнения alias.

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
- `godotenv v1.5.1` — чтение формата конфигурации из `v1.0.0` при миграции;
  `yaml.v3 v3.0.1` — алиасы и метаданные скриптов; `x/term` — терминал.
- `golang-migrate/migrate/v4 v4.20.1` — применение SQL-схемы истории;
  `modernc.org/sqlite v1.62.0` — SQLite-драйвер истории; `kljensen/snowball v0.10.0`
  и `x/text v0.43.0` — локальный русско-английский lexical-профиль.
  `CGO_ENABLED=0 make build-all` проверяет кросс-компиляцию четырёх Make-целей,
  но не native-поведение целевых ОС или полную матрицу GoReleaser.

Для реального запроса нужны доступный endpoint и настройки выбранного провайдера;
для Ollama — работающий локальный/удалённый совместимый endpoint.
Обычные unit-тесты не требуют провайдерских ключей или внешней LLM.

## Конфигурация и данные

Источник истины — [internal/core/config/config.go](internal/core/config/config.go).
`zev setup` сохраняет настройки в `~/.zev/config.yaml`. Переменные окружения не
переопределяют значения YAML. Старые имена `LLM_PROVIDER`, `ZEV_LANG` и другие
dotenv-ключи относятся только ко входу мигратора `v1.0.0`.

- `language`: `auto` по умолчанию; `ru`/`en` фиксируют язык служебного UI и help, а natural-language query определяет язык по текущему запросу.
- `active_provider` выбирает профиль из `providers`; настройки `type`, `model`,
  `api_key`, `base_url` принадлежат профилю. `http_headers` — YAML-отображение.
- `command_execution_mode` по умолчанию `inject`; CLI-флаги переопределяют настройку.
- `script_editor`, затем `EDITOR`, затем доступный `nano`/`vim`/`vi` определяют редактор;
  `SKIP_EDITOR=1` отключает его запуск. Редактор работает с приватной временной
  копией; результат атомарно заменяет скрипт после проверки исходной идентичности.
  Устаревшее редактирование не перезаписывает новую версию другого кооперативного
  редактора. Ошибка/отмена редактора оставляет уже зарегистрированный скрипт,
  временная копия очищается.
- `history_enabled` и `context_enabled` по умолчанию `true`; флаги `--no-history`
  и `--no-context` отключают соответствующее поведение для одного запроса.
  `tools_enabled` по умолчанию `true`, `tool_approval_mode` — `manual`; допустимы
  `manual`, `auto` и `bypass`, причём режим проверяется даже при отключённых инструментах.
  `security.dangerous_commands.add/remove` настраивают точные полные строки команд в локальном классификаторе;
  точное совпадение исключения снимает только соответствующее default finding и не отменяет structural-проверки.
  Отказ от interactive ask-подтверждения в query execute сохраняется в `security.denied_commands`,
  и такой exact deny имеет приоритет над bypass. В `--no-context` или `context_enabled: false` инструменты по умолчанию отключаются,
  но явный `--tools` переопределяет это ограничение. В `--print`/`--json` `--tools`
  требуется явно; machine mode не открывает TUI, а manual-вызов возвращает модели отказ.
  В интерактивном manual требуется буквальное `yes` в stderr; auto ограничен `pwd`,
  `ls`, `cat`, `head` с поддерживаемыми флагами, bypass предупреждает в stderr и
  не является sandbox или security guarantee. Tool-обмен эфемерен и не записывается
  в историю, но может увеличить токены/стоимость и раскрыть низкоуровневые секреты.
  Лимиты вызовов, таймаута и вывода фиксированы; native Windows runtime не проверен.
  `history_semantic_enabled` по умолчанию `false`; при включении требуется отдельный
  `openai-compatible` provider, base URL, model, positive dimensions, API key (если нужен)
  и `history_embedding_max_bytes` (default `32768`). Отключение истории не удаляет записи,
  не открывает SQLite и не отключает диалог в памяти.

| Путь | Формат и назначение |
| --- | --- |
| `~/.zev/config` | Dotenv из релиза `v1.0.0`; вход мигратора, не текущий конфиг |
| `~/.zev/config.yaml` | Текущая конфигурация и профили провайдеров (`version: 1`); файл с ключами создаётся с режимом `0600` |
| `~/.zev/aliases` | Текущий YAML (`version: 2`): шаблон, описание и аргументы алиаса |
| `~/.zev/scripts` | Текущий YAML (`version: 2`): ID записи, файл скрипта, описание и аргументы |
| `~/.zev/script_files/*.sh` | Тела bash-скриптов; при создании режим `0700` |
| `~/.zev/history.db` | SQLite с постоянными диалогами, сообщениями, ответами, командами и индексами; без лимита количества |
| `~/.zev/history` | Legacy JSONL, consumed once by the migrator and then deleted |
| `~/.zev/history.migrated.*.jsonl` | Not created; migrated records live only in SQLite |

Query recorder сохраняет первоначальный запрос, каждое уточнение, ответы, варианты
команд, выбор и наблюдаемый результат. Выбор/вставка не означают выполнение; аварийный
выход может оставить `started` и неизвестный outcome. Startup истории в CLI запускает
`history.Migrate` из `internal/migrations/history` под lock: проверяет fingerprint и количество, удаляет источник
после успешной активации, затем завершает ledger. Миграции не создают архивы или резервные копии
и не обходят/удаляют исторические пользовательские копии; гарантия безопасного стирания не даётся.
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
Удаление логическое, не гарантирует forensic erase WAL/free pages, backup или удалённых копий.
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
Ошибки очистки тоже могут сопровождать выполненное изменение. Изменения файлов
метаданных и тела скрипта координируются с попыткой не публиковать неполную пару до
фиксации, но это не rollback и не crash-atomic транзакция над двумя файлами.
`internal/migrations/filetxn` относится к файловым миграциям, а не к CRUD скриптов.
На Windows directory-fsync не выполняется; Unix mode bits не являются доказательством
настроенных Windows ACL.

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
  Тесты lifecycle, migration/recovery, транзакций, lexical/vector search, CLI и embedding
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

Unit-тесты находятся рядом с кодом в `*_test.go`, включая `internal/cli`, используют
`testing` и локальные заглушки/`httptest`. Сквозные сценарии CLI и интеграционные
сценарии установщиков/релиза находятся в `e2e/` с тегом `e2e`; `make test-e2e` собирает
настоящий CLI в отдельном owned-каталоге внутри `bin/` и запускает все e2e-пакеты с
изолированным HOME и локальными HTTP/shell fixtures. `make test-integration` оставлен
совместимым псевдонимом для этого gate. Обычный `make test` не требует shell-инструментов
для deployment fixtures. Этот gate запускается в CI и перед release publication.
`make test-manual ALLOW_MANUAL_TESTS=1` требует отдельного разрешения и запрещён при
заданном `CI`.
Существующего target для генерации исходников нет.

## Сборка и выпуск

[.goreleaser.yaml](.goreleaser.yaml) описывает релизные сборки без CGO:
Linux amd64/arm64/armv7, macOS amd64/arm64, Windows amd64/arm64; архивы `tar.gz`,
для Windows — `zip`, плюс `checksums.txt`. Версия внедряется в `main.version`;
GoReleaser уже использует `-s -w` и `-trimpath`.

[GitHub workflow](.github/workflows/release.yml) запускается при изменении `VERSION`
в `main` или вручную и устанавливает закреплённые версии релизных инструментов.
Он и `make release` используют [deploy/release.sh](deploy/release.sh):
`make fmt`, `make lint`, `make test`, `make test-race`, `make test-e2e`, `make build VERSION=<version>`
выполняются до создания или push тега. До hooks и после build/GoReleaser проверяются
и tracked изменения, и untracked relevant source (включая `.go`); такой source блокирует
выпуск. Изменения отслеживаемых файлов относительно
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

`zev update` по умолчанию использует последний стабильный GitHub Release;
`zev update main` явно выбирает последний коммит `main`. Stable проверяет SHA-256
и протокол совместимости кандидата, а main собирает исходники локально до замены
исполняемого файла. После установки новый бинарник запускает все локальные
миграторы; актуальная версия тоже повторяет миграции. `zev update --migrate-only`
выполняет их без сети. Ошибка после установки сохраняет новую версию: автоматический
откат несовместим с уже мигрированными данными. Нужны права записи в каталог
бинарника; автоматического `sudo` нет. Подробности и границы — в
[руководстве по обновлению](docs/ru/UPDATE.md).

- `inject` использует TIOCSTI на Linux/macOS и требует TTY и разрешения ОС.
  При отказе или на Windows CLI печатает команду вместо вставки.
- `execute` выбирает **первую подсказку** и применяет локальный shell-анализ, отдельный
  provider review для неизвестного синтаксиса, security policy и при необходимости
  подтверждение до запуска. В `security.mode: bypass` confirmation пропускается для
  допустимой host-доставки, но deny rules и технические проверки остаются.
  `inject` — warning-only: вставляет текст без Enter и не контролирует последующий запуск.
  Алиасы используют проверенный POSIX-совместимый shell; скрипты — Bash на Unix.
  Исполнение алиасов и скриптов в Windows ограничено/не поддерживается.
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
- CLI/TUI используют именованные ключи из `internal/core/locale/keys.go`; английские и русские
  тексты находятся в TOML-каталогах `locale/`, встраиваемых через корневой `embed.go`, а не в местах вызова.

Это описание существующих ограничений, а не разрешение ослаблять проверки или
расширять scope изменений. Изменение исполнения команд, контекстов или файловых
контрактов требует отдельной реализации и соответствующих тестов.

## Контракты и документация

- [README.ru.md](README.ru.md) / [README.md](README.md) — использование и установка.
- [Конфигурация](docs/ru/CONFIGURATION.md) — параметры и файлы данных.
- [Безопасность](docs/ru/SECURITY.md) — реализованное поведение policy, tools, web fetch и приватности.
- [Алиасы и скрипты](docs/ru/ALIASES-SCRIPTS.md) — текущий формат, валидация, публикация и выполнение.
- [Обёртка `zev run`](docs/ru/RUN-WRAPPER.md) — literal argv, recovery и границы процесса.
- [История](docs/ru/HISTORY.md) — постоянное хранилище, миграция, поиск и semantic opt-in.
- [Архитектура](docs/ru/ARCHITECTURE.md) — пакеты и порт провайдера.
- [Разработка](docs/ru/DEVELOPMENT.md) и [участие](CONTRIBUTING.ru.md) — команды и правила.
- [CHANGELOG.md](CHANGELOG.md) — пользовательские изменения по существующим релизам.
- Go-контракт провайдера — [domain/provider.go](internal/core/domain/provider.go),
  JSON ответа — [domain/command.go](internal/core/domain/command.go).
- Контракт истории — [history/service/types.go](internal/entities/history/service/types.go),
  SQLite — [history/repository/store.go](internal/entities/history/repository/store.go),
  lexical — [history/lexical/lexical.go](internal/entities/history/lexical/lexical.go),
  embedding client — [history/client/client.go](internal/entities/history/client/client.go).

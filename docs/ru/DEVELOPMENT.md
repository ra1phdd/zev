<div align="center">

[English](../en/DEVELOPMENT.md) | **Русский**

</div>

# Разработка

## Требования

- Go 1.27.2 или новее
- `gofmt` (в составе Go), `gofumpt`, `goimports`,
  [golangci-lint](https://golangci-lint.run/) v2 и `deadcode` для `make lint`
- GoReleaser v2 для проверки релизной конфигурации и snapshot-сборок
- Опционально `entr` для `make watch`

## Подготовка

```bash
git clone https://github.com/ra1phdd/zev.git
cd zev
make build
```

## Make-цели

| Цель | Описание |
|---|---|
| `make build` | Сборка в `bin/zev` |
| `make install` | `go install` в `$GOPATH/bin` |
| `make test` | `go vet ./...`, затем `go test ./...` |
| `make test-race` | `go test -race ./...` |
| `make test-integration` | Тесты с build tag `integration` |
| `make test-e2e` | Тесты с build tag `e2e` |
| `make test-manual ALLOW_MANUAL_TESTS=1` | Явно разрешённые manual-тесты; в CI запрещены |
| `make lint` | Проверка gofmt/gofumpt/goimports, golangci-lint с автоисправлениями, отчёт deadcode для `APP` |
| `make fmt` | Применить gofmt, goimports и gofumpt |
| `make check` | Линтер + тесты |
| `make run` | Сборка и запуск интерактивного режима |
| `make setup` | Сборка и запуск мастера настройки |
| `make dev` | Запуск с debug-логами |
| `make watch` | Перезапуск при изменениях (нужен `entr`) |
| `make build-all` | Кросс-компиляция под Linux, macOS и Windows |
| `make clean` | Удалить `BIN` и четыре кросс-сборки, сохранив остальные файлы |
| `make release-check` | Проверить `.goreleaser.yaml` без публикации |
| `make release-snapshot` | Собрать snapshot через GoReleaser без публикации |
| `make version` | Показать версию сборки (по умолчанию `dev`) |
| `make info` | Показать пакет приложения, путь бинарника, версию и платформу Go |

Переменные `GO`, `APP`, `BIN`, `FLAGS` и `VERSION` можно переопределить. `BIN` должен
указывать на файл внутри `bin/`, без компонентов пути `..`. `make clean`
проверяет тот же путь. `ARGS` передаёт аргументы в `run`, `setup` и `dev`:

```bash
make run ARGS=version
make build VERSION=1.0.0 BIN=bin/zev-local
make clean BIN=bin/zev-local
```

`make install` напрямую вызывает `go install`, учитывая `GOBIN` или `GOPATH/bin`,
без лишней локальной сборки. `make dev` включает debug-логи через `-vv`.
Локальная цель `make build-all` собирает Linux amd64, macOS amd64/arm64 и
Windows amd64. GoReleaser дополнительно собирает Linux arm64/armv7 и Windows
arm64; его архивы и snapshot-артефакты сейчас размещаются в `dist/`.

Локальные сборки передают линкеру Go `-s -w`: таблица символов и отладочная
информация DWARF удаляются для уменьшения бинарника, а `-X main.version`
сохраняется. `FLAGS` также содержит `-buildid=` для удаления Go build ID
и `-trimpath` для удаления локальных путей. GoReleaser уже использует `-s -w`
и `-trimpath`. Эти флаги не включают
дополнительные оптимизации CPU; для отладки на уровне исходников нужна
сборка без удаления символов:

```bash
make build FLAGS=
```

`make lint` запускает `golangci-lint run --fix` и может менять Go-файлы.
`deadcode -test $(APP)` показывает достижимость для выбранного приложения
и его тестов, но не доказывает возможность удаления функций на всех платформах.

Для локальных сборок `VERSION` по умолчанию равен `dev`; файл `VERSION` задаёт
версию релиза. `make release` создаёт или заменяет локальный тег версии и
публикует релиз через GoReleaser: это не проверочная команда. GitHub workflow
релиза запускается при изменении `VERSION` в `main` либо вручную и не запускает
тесты и линтер. Выполняйте эти проверки до релиза.

## TDD

Каждая фича пишется по правилу red-green-refactor:

1. Написать тест, описывающий требуемое поведение — он должен падать.
2. Написать минимальный код, который заставит его пройти.
3. Отрефакторить, сохраняя зелёные тесты.

```bash
make check
```

Коммитить код с незакрытыми замечаниями линтера запрещено.

## Тесты

- Юнит-тесты быстрые и изолированные, лежат рядом с проверяемым кодом.
- Integration-тесты используют build tag `integration`:

```bash
make test-integration
```

В текущих отслеживаемых тестах нет тегов `integration`, `e2e` или `manual`;
цели резервируют процесс для будущих tagged-тестов и также включают обычные
unit-тесты. Unit-тестам не нужны ключи провайдеров или запущенная LLM. Для
проверки гонок отдельно запускайте `make test-race`. Manual-тесты требуют
явного разрешения.

## Соглашения проекта

Профиль репозитория — в [PROJECT.md](../../PROJECT.md). Локальные правила
агентов находятся в `AGENTS.md` и `CLAUDE.md`. Кратко:

- `cmd/*` — только composition root; бизнес-логика живёт в
  `internal/entities/*/service`.
- Vertical slices плюс ports and adapters; интерфейсы объявляет потребитель.
- Никаких однострочных обёрток и абстракций «на будущее».
- Функции с более чем тремя аргументами принимают именованную структуру параметров,
  `context.Context` всегда первый и никогда не внутри структуры.
- Текст CLI сейчас находится в пакетах команд и TUI; пакета
  `internal/core/i18n` нет.

## Архитектура

Смотрите [ARCHITECTURE.md](ARCHITECTURE.md) — карта пакетов и путь запроса.

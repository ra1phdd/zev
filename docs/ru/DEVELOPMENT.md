<div align="center">

[English](../en/DEVELOPMENT.md) | **Русский**

</div>

# Разработка

## Требования

- Go 1.27.2 или новее
- [golangci-lint](https://golangci-lint.run/) для линтинга
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
| `make test` | `go test ./... -race` |
| `make lint` | `golangci-lint run --fix` |
| `make check` | Линтер + тесты |
| `make run` | Сборка и запуск интерактивного режима |
| `make setup` | Сборка и запуск мастера настройки |
| `make dev` | Запуск с debug-логами |
| `make watch` | Перезапуск при изменениях (нужен `entr`) |
| `make build-all` | Кросс-компиляция под Linux, macOS и Windows |
| `make clean` | Очистка артефактов сборки |

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
go test -tags integration ./...
```

## Соглашения проекта

Полные правила — в [AGENTS.md](../../AGENTS.md) и [CLAUDE.md](../../CLAUDE.md).
Кратко:

- `cmd/*` — только composition root; бизнес-логика живёт в
  `internal/entities/*/service`.
- Vertical slices плюс ports and adapters; интерфейсы объявляет потребитель.
- Никаких однострочных обёрток и абстракций «на будущее».
- Функции с более чем тремя аргументами принимают именованную структуру параметров,
  `context.Context` всегда первый и никогда не внутри структуры.
- Весь пользовательский текст проходит через `internal/core/i18n` (для самого CLI пока
  не подключён, но правило действует для новых пакетов).

## Архитектура

Смотрите [ARCHITECTURE.md](ARCHITECTURE.md) — карта пакетов и путь запроса.

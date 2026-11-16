<div align="center">
<h1>Zev</h1>
<p>Подсказки команд терминала на естественном языке — опишите задачу, получите команду</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat" alt="Платформы">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="Лицензия">
</p>

<img src="assets/zev.gif" width="346" alt="Zev в действии">

[English](README.md) | **Русский**
</div>

## Возможности

- Естественный язык на входе, команды терминала на выходе — до трёх вариантов с кратким
  пояснением к каждому.
- Опасные операции (`rm -rf`, `DROP TABLE`, `kubectl delete`) помечаются предупреждением с
  описанием того, что именно сделает команда.
- Пять LLM-провайдеров: OpenAI, Gemini, Anthropic, Ollama и OpenCode.
- Пользовательский base URL — работает с любым OpenAI-совместимым роутером.
- Интерактивный TUI-селектор: выберите подсказку стрелками, она попадёт в терминал или
  выполнится сразу.
- Алиасы команд с подстановкой позиционных аргументов — `zev dps my-service` выполнит вашу
  длинную сохранённую команду.
- Bash-скрипты с метаданными, создаются прямо из CLI в `$EDITOR`.
- Постоянная история запросов доступна офлайн в `~/.zev/history.db`, без автоматического
  ограничения количества записей.
- Локальный lexical-поиск с русским/английским stemming, ограниченными опечатками и
  exact-command-фильтрами.
- Semantic/hybrid-поиск в сервисе (CLI-интеграция ещё не завершена); embeddings включаются
  явно и применяются только к пользовательским запросам и уточнениям.
- Уровень логирования через `-v`, `-vv`, `-vvv`.

## Быстрый старт

### Linux и macOS

Установите последний релиз одной командой — скрипт сам подберёт архив под вашу ОС и
архитектуру. Установка продолжается только если доступны `checksums.txt` и SHA-256-утилита,
файл содержит ровно одну корректную запись для скачанного архива, а хеш совпадает:

```bash
# curl
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash

# или wget
wget -qO- https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
```

Оба варианта ставят в `/usr/local/bin` через `sudo`, если он есть, а без прав root — в
`$HOME/.local/bin`. Чтобы выбрать каталог самостоятельно, укажите `PREFIX`:

```bash
PREFIX=$HOME/.local/bin curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
```

Скрипт использует `curl`, если он есть, иначе `wget`. Чтобы поставить конкретную версию,
передайте `-v`:

```bash
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash -s -- -v 1.0.0
```

### Windows

```powershell
irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.ps1 | iex
```

Или скачайте [`install.ps1`](deploy/install.ps1) и запустите `.\install.ps1`. Ставит в
`%LOCALAPPDATA%\Programs\zev`, права администратора не нужны. Проверка SHA-256 здесь
тоже обязательна: отсутствие или некорректность checksums и несовпадение хеша
останавливают установку.

### Затем настройте провайдера

```bash
zev setup                          # интерактивная настройка LLM-провайдера
zev "найти все файлы, изменённые за последние 24 часа"
```

Без подкоманды `query` запускается интерактивный режим.

## Использование

### Запрос

```bash
zev "убить процесс, слушающий порт 8080"
zev query "показать все docker-контейнеры по размеру"
```

### Что делать с выбранной командой

```bash
zev --inject "сжать логи"      # подставить в терминал для проверки (по умолчанию)
zev --execute "перезапустить nginx"   # выполнить сразу
```

Поведение по умолчанию берётся из `COMMAND_EXECUTION_MODE` и меняется через
`zev setup mode`.

### История

> SQLite-история — Unreleased. Автоматическая миграция, opt-in семантический поиск
> и явное обслуживание индексов описаны в [гайде по истории](docs/ru/HISTORY.md).

```bash
zev history
zev history search "большие файлы" --limit 20
zev history search "network error" --mode hybrid --json
zev history search --exact-command 'git status --short'
zev history show <conversation-id>
zev history index status --json
zev history index rebuild --fts
zev history export --format json
zev history delete <conversation-id>
zev history clear --yes
```

История хранится в SQLite в `~/.zev/history.db`: диалог записывается по мере работы,
автоматической ротации по количеству нет. Query с историей и явные команды истории
автоматически импортируют и архивируют старый `~/.zev/history` JSONL после проверки.
`--no-history` и `HISTORY_ENABLED=false` не открывают хранилище и не запускают миграцию
для текущего запроса. По умолчанию поиск локальный лексический; semantic/hybrid
использует отдельно настроенные opt-in embeddings и явно построенные векторы
user-сообщений. Vector rebuild требует согласия `--vectors --yes`; векторы и RAG-контекст
не создаются автоматически. JSON-поиск включает покрытие, усечение и `nextCursor` для
продолжения через `--cursor`. Удаление не стирает legacy-архивы, backup и внешние копии. См.
[гайд по истории](docs/ru/HISTORY.md): миграция, режимы поиска, приватность и embeddings.

### Алиасы

Сохраните длинную команду один раз и вызывайте по имени. Лишние аргументы
дописываются автоматически либо подставляются в шаблон:

```bash
zev alias add dps "docker service ps {{.arg1}} --no-trunc"
zev dps my-service

zev alias list
zev alias remove dps
```

Переменные шаблона: `{{.arg1}}`, `{{.arg2}}`, … — позиционные аргументы, `{{.args}}` —
все аргументы через пробел.

### Скрипты

```bash
zev script create deploy "Деплой приложения в продакшен"
zev script list
zev script remove deploy
```

Метаданные хранятся в `~/.zev/scripts`, тела скриптов — в `~/.zev/script_files`.
`zev script create` открывает файл в `$SCRIPT_EDITOR`, `$EDITOR` или первом доступном из
`nano`, `vim`, `vi`.

### Прочее

```bash
zev version
zev -vv "смотреть логи в реальном времени"   # debug-логи
```

## Провайдеры

| Провайдер | `LLM_PROVIDER` | Обязательные переменные |
|---|---|---|
| OpenAI | `openai` | `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` |
| Gemini | `gemini` | `GEMINI_API_KEY`, `GEMINI_MODEL` |
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` |
| Ollama | `ollama` | `OLLAMA_BASE_URL`, `OLLAMA_MODEL` |
| OpenCode | `opencode` | `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` |

Ollama работает полностью локально:

```bash
export LLM_PROVIDER=ollama
export OLLAMA_BASE_URL=http://localhost:11434/v1
export OLLAMA_MODEL=qwen2.5-coder
```

## Документация

- [Конфигурация](docs/ru/CONFIGURATION.md) — `~/.zev/config`, переменные окружения, режимы
  выполнения, файлы на диске
- [История](docs/ru/HISTORY.md) — постоянная SQLite-история, миграция, поиск и semantic opt-in
- [Архитектура](docs/ru/ARCHITECTURE.md) — карта пакетов, путь запроса, контракты
  провайдеров
- [Разработка](docs/ru/DEVELOPMENT.md) — make-цели, TDD, соглашения проекта
- [Contributing](docs/ru/CONTRIBUTING.md) — как предложить и принять изменение
- [Профиль проекта](PROJECT.md) — факты о репозитории и эксплуатационные ограничения
- [Changelog](CHANGELOG.md) — история релизов

## Технологии

- Go 1.27+, Cobra для CLI
- Bubble Tea + Lipgloss для терминального UI
- [go-openai](https://github.com/sashabaranov/go-openai) для OpenAI-совместимых
  провайдеров, [go-anthropic](https://github.com/liushuangls/go-anthropic) для
  Anthropic, raw HTTP для Gemini и OpenCode (SSE streaming)
- `godotenv` + `caarlos0/env` для конфигурации
- `golangci-lint`, `go vet` и `go test` через Make-цели

## Удаление

Чтобы удалить zev, воспользуйтесь скриптами:

```bash
# Linux и macOS
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.sh | bash
```

```powershell
# Windows
irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.ps1 | iex
```

Или удалите бинарник вручную:

```bash
rm /usr/local/bin/zev      # системная установка (Linux/macOS)
rm $HOME/.local/bin/zev    # пользовательская (Linux/macOS)
```

```cmd
REM Windows
del %LOCALAPPDATA%\Programs\zev\zev.exe
```

## Contributing

Приветствуем вклад — читайте [гайд по участию](docs/ru/CONTRIBUTING.md).

## Лицензия

Проект лицензирован по MIT License — см. файл [LICENSE](LICENSE).

<div align="center">

[English](../en/CLI.md) | **Русский**

</div>

# Справочник CLI

Здесь приведено полное дерево команд и собственные флаги каждой команды. Справочник основан на определениях Cobra, исходном коде и runtime-выводе `--help`; скрытые и внутренние флаги помечены отдельно. Команды `zev help` нет: используйте `zev --help`, `zev -h` или добавьте `--help`/`-h` к команде. В документированном CLI отдельная команда `help`, отображавшаяся в старых сборках, удалена.

## Обозначения и наследуемые флаги

- `[значение]` — необязательный аргумент; `<значение>` — обязательный; `...` — повторение/оставшиеся аргументы.
- Если не указано иное, логические флаги по умолчанию `false`, строки пустые. `--help` (`-h`) есть у каждой команды Cobra и показывает справку, не выполняя действие.
- **Наследуемые флаги обычных подкоманд:** `--proxy URL` (по умолчанию пусто; направляет сетевые запросы Zev через SOCKS5 или HTTP(S)) и `-v, --verbose` (счётчик, по умолчанию `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE). Они перечислены в записи каждой команды.
- **Режим корневого запроса:** корень разбирает запрос вручную и сохраняет нераспознанные флаги для алиасов/скриптов. Также у корневого запроса есть флаги инструментов `--tools`, `--no-tools`, `--tool-approval MODE`; см. `zev [query]` ниже. Для справки Cobra используйте `zev --help`, для флагов запроса — `zev query --help`.
- У `zev run` отдельный разбор literal argv (`DisableFlagParsing`): `--proxy` там является прокси для восстановления после ошибки, а опции задаются до `--`.

## Дерево команд

```text
zev [query]
├── alias
│   ├── add [name template [description [arguments-json]]]
│   ├── edit [name]
│   ├── last <alias-name>
│   ├── list
│   └── remove [name] (алиасы: rm, delete)
├── completion
│   ├── bash
│   ├── fish
│   ├── powershell
│   └── zsh
├── continue [clarification]
├── doctor
├── history
│   ├── clear
│   ├── delete <id>
│   ├── export
│   ├── index
│   │   ├── rebuild
│   │   └── status
│   ├── search [query]
│   └── show [conversation-id]
├── provider
│   ├── add NAME
│   ├── list
│   ├── remove NAME
│   └── use NAME
├── query [query]
├── run -- command [args...]
├── script
│   ├── create [name [description [arguments-json]]]
│   ├── edit [name]
│   ├── list
│   ├── remove [name] (алиасы: rm, delete)
│   └── show <name>
├── setup
│   ├── headers
│   └── mode [inject|execute]
├── update [stable|main]
└── version
```

Корневой режим `zev [query]` и явный `zev query` используют один путь запроса. Встроенные имена команд приоритетнее алиасов и скриптов. В остальных случаях имя алиаса/скрипта вызывается напрямую, аргументы идут после имени; аргументы алиаса располагайте в конце вызова. Например: `zev list-containers --format json`.

## Корневой запрос: `zev [query]`

Принимает ноль или несколько слов запроса, объединяемых в prompt. Без запроса открывается интерактивный ввод. Флаги запроса разбираются вручную и могут находиться рядом с текстом запроса.

**Локальные флаги корня:**

| Флаг | Значения/умолчание | Действие |
|---|---|---|
| `-h, --help` | — | Показать справку корневой команды. |
| `--execute`, `-e` | boolean; `false` | Сразу выполнить выбранную подсказку; несовместим с `--inject`. |
| `--inject` | boolean; `false` | Вставить выбранную команду в терминал; режим по умолчанию, если конфигурация не задаёт другой. |
| `--no-history` | boolean; `false` | Не сохранять запрос. |
| `--no-context` | boolean; `false` | Не отправлять локальный контекст окружения. |
| `--tools` | boolean; `false` | Включить model context tools; требуется явно вместе с `--print`/`--json`. |
| `--no-tools` | boolean; `false` | Отключить model context tools. |
| `--tool-approval MODE` | `manual`, `auto`, `bypass`; иначе настройка из config | Задать режим согласования вызовов инструментов. |
| `--print` | boolean; `false` | Напечатать выбранную команду без выполнения (machine mode). |
| `--json` | boolean; `false` | Выдать JSON вариантов команд схемы version 1 (machine mode). |
| `--option N` | integer; по умолчанию `1` | Выбрать вариант команды с индексом от 1 в machine mode. |
| `--context-file`, `-cf PATH` | повторяемый; нет | Прикрепить файл контекста проекта. |
| `--allow-sensitive-context` | boolean; `false` | Разрешить явно запрошенные чувствительные пути контекста; symlink запрещены. |
| `--proxy URL` | string; пусто | Направить запросы к провайдеру/сети через SOCKS5 или HTTP(S)-прокси. |
| `-v, --verbose` | счётчик; `0` | Увеличить уровень логов (`-v` INFO, `-vv` DEBUG, `-vvv` TRACE). |

Неизвестные флаги сохраняются для именованного alias/script; `--` передаёт оставшиеся токены буквально. Парсер отдельно распознаёт help, verbosity, режим выполнения, настройки tools, proxy, machine output и context-file. Это не обычный разбор позиционных аргументов Cobra.

**Типичные примеры:**

```sh
zev "find files larger than 100 MB"
zev query "show running containers"
zev --print --json "list current branch"
```

## `zev query [query]`

Явное написание корневого режима запроса. Принимает ноль или больше токенов запроса; нераспознанные флаги сохраняются для алиасов при dispatch. Локальные флаги и значения те же, что у `zev [query]`; дополнительно наследуются `--proxy`, `-v, --verbose` и help. `--execute` несовместим с `--inject`; `--option` в machine mode начинается с 1 и по умолчанию равен 1. `--print` печатает выбранную команду, `--json` печатает схему вариантов. Для использования tools в machine mode требуется явный `--tools`.

## `zev continue [clarification]`

Продолжает последний пригодный сохранённый диалог. Оставшиеся слова — уточнение (ноль или больше). Команда намеренно отключает разбор флагов Cobra во время выполнения, но использует тот же контракт query-флагов для справки и проверки; позиционный текст считается уточнением. `--no-history` распознаётся парсером, но отклоняется, потому что continuation требует историю.

**Локальные флаги:** `--execute`, `--inject`, `--no-history`, `--no-context`, `--continue`, `--print`, `--json`, `--option N`, `--context-file PATH` (повторяемый), `--allow-sensitive-context`, `--tools`, `--no-tools`, `--tool-approval MODE` (`manual|auto|bypass`), `-h, --help`. **Наследуемые:** `--proxy URL`, `-v, --verbose`. Специальный query parser распознаёт эти флаги; `--no-history` отклоняется, потому что continuation требует историю.

## Алиасы: `zev alias`

Управление шаблонами команд. Шаблон принимает одну простую команду и поддерживает `{{.arg1}}`, `{{.arg2}}`, … и `{{.args}}`; для пайпов и перенаправлений используйте скрипт. `zev alias last NAME` сохраняет последнюю подходящую команду из истории как новый алиас.

### `zev alias add [name template [description [arguments-json]]]`

Создать алиас; пропущенные значения можно ввести в интерактивной форме. Допускается от 0 до 4 позиционных аргументов. Для metadata JSON аргументов — упорядоченный JSON-массив; `--argument NAME[=DESCRIPTION]` можно повторять. **Алиасов нет. Локальные флаги:**

- `--argument NAME[=DESCRIPTION]` (string array, повторяемый; изначально пусто): задать метаданные обязательного аргумента.
- `--arguments-json JSON` (string, пусто): заменить упорядоченный список аргументов из JSON.
- `--clear-arguments`, `--clear-description` (boolean, `false`): очистить соответствующие метаданные.
- `--description TEXT` (string, пусто): заменить описание.
- `--generate-arguments`, `--generate-description` (boolean, `false`): явно разрешить генерацию метаданных через AI.
- `--json` (boolean, `false`): schema-versioned JSON без prompts; включает неинтерактивное поведение.
- `--non-interactive` (boolean, `false`): отключить prompts/editors.
- `--template ARGV-TEMPLATE` (string, пусто): задать/заменить argv-safe шаблон, когда имя передано позиционно или введено в форме.
- `--no-metadata` (boolean, `false`): создать алиас без metadata.
- `-h, --help`.

Наследуемые: `--proxy URL` (пусто), `-v, --verbose` (счётчик, `0`). Укажите один источник шаблона: позиционный template или `--template`.

### `zev alias edit [name]`

Редактировать алиас с указанным именем или выбрать его интерактивно; допускается 0–1 позиционный аргумент. **Локальные флаги:** `--argument NAME[=DESCRIPTION]` (повторяемый string array, default пусто), `--arguments-json JSON` (string, пусто), `--clear-arguments` (boolean, `false`), `--clear-description` (boolean, `false`), `--description TEXT` (string, пусто), `--generate-arguments` (boolean, `false`), `--generate-description` (boolean, `false`), `--json` (boolean, `false`), `--no-metadata` (boolean, `false`), `--non-interactive` (boolean, `false`), `--template ARGV-TEMPLATE` (string, пусто), `-h, --help`. **Наследуемые:** `--proxy URL` (по умолчанию пусто), `-v, --verbose` (счётчик, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Конфликтующие источники metadata отклоняются.

### `zev alias last <alias-name>`

Сохранить последнюю использованную команду из истории под обязательным именем алиаса (ровно один позиционный аргумент). **Локальные флаги:** `--argument NAME[=DESCRIPTION]` (повторяемый string array, default пусто), `--arguments-json JSON` (string, пусто), `--clear-arguments` (boolean, `false`), `--clear-description` (boolean, `false`), `--description TEXT` (string, пусто), `--generate-arguments` (boolean, `false`), `--generate-description` (boolean, `false`), `--json` (boolean, `false`), `--no-metadata` (boolean, `false`), `--non-interactive` (boolean, `false`), `--template ARGV-TEMPLATE` (string, пусто), `-h, --help`. **Наследуемые:** `--proxy URL` (по умолчанию пусто), `-v, --verbose` (счётчик, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. В истории должна быть пригодная последняя команда.

### `zev alias list`

Показать алиасы; позиционных аргументов нет. **Локальные:** `--json` (boolean, `false`; schema-versioned JSON без prompts), `--non-interactive` (boolean, `false`), help. **Наследуемые:** proxy и verbosity.

### `zev alias remove [name]` (`rm`)

Удалить алиас по необязательному имени; без имени используется интерактивный выбор. **Алиасы:** `rm`, `delete`. **Локальные:** `--json` (boolean, `false`), `--non-interactive` (boolean, `false`), `--yes` (boolean, `false`; подтвердить удаление именованной командой), help. **Наследуемые:** `--proxy URL` (по умолчанию пусто), `-v, --verbose` (счётчик, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

## Shell completion: `zev completion`

`zev completion` не принимает позиционных аргументов. **Локальные флаги родителя:** `-h, --help`. **Наследуемые:** `--proxy URL` (по умолчанию пусто), `-v, --verbose` (счётчик, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Каждая shell-команда не принимает дополнительных позиционных аргументов и имеет **локальные флаги:** `--no-descriptions` (boolean, default `false`; убрать описания вариантов), `-h, --help`. **Наследуемые на каждом shell-пути:** `--proxy URL` (по умолчанию пусто), `-v, --verbose` (счётчик, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

- `zev completion bash`: создаёт Bash completion; требуется пакет `bash-completion`.
- `zev completion fish`: создаёт Fish completion.
- `zev completion powershell`: создаёт PowerShell completion.
- `zev completion zsh`: создаёт Zsh completion.

Каждая команда выводит текст скрипта и не устанавливает shell-настройки автоматически. Инструкции установки: `zev completion SHELL --help`.

## `zev doctor`

Проверить локальную конфигурацию и готовность провайдера. Позиционных аргументов нет. **Локальные:** `--json` (boolean, `false`; стабильный JSON-отчёт), `--print` (boolean, `false`; человекочитаемый отчёт без prompts), `--confirm` (boolean, `false`; отправить один реальный запрос провайдеру без подтверждения), help. **Наследуемые:** `--proxy URL` (пусто), `-v, --verbose` (счётчик, `0`). `--confirm` может создать сетевой запрос.

## История: `zev history`

Без аргументов открывает браузер истории и после выбора показывает диалог в человекочитаемом виде. Браузер использует ограниченную страницу и предлагает показать более старые диалоги, если они есть. С `--json` интерактивный выбор не запускается, выводится JSON-массив кратких записей диалогов. Локальные флаги: `--json` (boolean, `false`; вывести краткие записи в JSON), help. Наследуются proxy и verbosity.

### `zev history search [query]`

Поиск с нулём или одним аргументом query; без запроса используется интерактивный режим. Режимы: lexical (по умолчанию), semantic, hybrid.

**Локальные флаги:** `--mode MODE` (`lexical|semantic|hybrid`, default `lexical`); `--limit N` (integer, default `20`); `--cursor TOKEN` (string, пусто; продолжить предыдущую страницу); `--from TIME` (string, включительная UTC-дата или RFC3339 time); `--to TIME` (string, исключительная UTC-дата или RFC3339 time); `--provider NAME`, `--model NAME`, `--status STATUS`, `--type TYPE` (string, пусто; фильтры); `--dangerous` (boolean, `false`); `--fuzzy MODE` (`auto|always|off`, default `auto`); `--exact-command TEXT` (string, пусто); `--literal` (boolean, `false`); `--json` (boolean, `false`); help. **Наследуемые:** proxy и verbosity. Для cursor продолжения сохраняйте тот же запрос, лимит и фильтры; покрытие результата указывает на усечение.

### `zev history show [conversation-id]`

Показать диалог и, при необходимости, сообщение в человекочитаемом виде. Не более одного ID диалога. **Локальные:** `--message ID` (string, пусто; показать сообщение по полному ID), `--json` (boolean, `false`; вывести исходный объект в JSON), help. **Наследуемые:** proxy и verbosity.

### `zev history export`

Экспортировать всю историю; позиционных аргументов нет. **Локальные:** `--format FORMAT` (`json|jsonl`, default `json`), help. **Наследуемые:** proxy и verbosity.

### `zev history delete <id>`

Удалить один диалог по ID (ровно один аргумент). Локальные флаги: только help. Наследуемые: proxy и verbosity. Удаление логическое и не гарантирует безопасное стирание.

### `zev history clear`

Удалить все диалоги; аргументов нет. **Локальные:** `--yes` (boolean, `false`; подтвердить удаление), help. **Наследуемые:** proxy и verbosity.

### `zev history index`

Группа обслуживания индексов; позиционных аргументов нет. Локальный флаг: help. Наследуются proxy и verbosity.

#### `zev history index status`

Показать состояние индексов; аргументов нет. **Локальные:** `--json` (boolean, `false`), help. **Наследуемые:** proxy и verbosity.

#### `zev history index rebuild`

Перестроить индексы; аргументов нет. **Локальные:** `--fts` (boolean, `false`; перестроить локальные лексические индексы), `--vectors` (boolean, `false`; перестроить semantic-векторы), `--retry-failed` (boolean, `false`; повторить неудачные векторные сообщения), `--yes` (boolean, `false`; согласие на передачу текста для векторизации), help. **Наследуемые:** proxy и verbosity. Требуется ровно один флаг из `--fts` и `--vectors`; для vector rebuild обязателен `--yes`. `--retry-failed` относится к vector rebuild.

## Профили провайдеров: `zev provider`

Управление именованными LLM-профилями. У родителя нет позиционных аргументов; доступны help и наследуемые proxy/verbosity.

- `zev provider list`: без аргументов и локальных флагов, кроме help; показывает имена/типы и отмечает активный профиль. Наследуются proxy, verbosity.
- `zev provider use NAME`: ровно одно имя профиля; локально только help. Наследуются proxy, verbosity.
- `zev provider add NAME`: ровно одно имя профиля; запускает интерактивную настройку профиля. Локально только help. Proxy используется для запросов к провайдеру; verbosity наследуется.
- `zev provider remove NAME`: ровно одно имя профиля; локально только help. Наследуются proxy, verbosity. Нельзя удалить активный профиль.

## `zev run -- command [args...]`

Запустить literal argv и проксировать стандартные потоки. Всё после `--` передаётся дочернему процессу без shell parsing; разделитель обязателен. За один вызов Zev делает **одну попытку**; параметры количества/задержки повторов и retry exit code не поддерживаются. После сбоя доступно интерактивное восстановление, анализ или предложение исправления; предложения автоматически не выполняются. Захваченный вывод, отправляемый провайдеру на анализ, может содержать секреты.

Run-опции должны стоять до `--`. **Локальные опции:**

| Опция | Значения/умолчание | Назначение |
|---|---|---|
| `--on-failure=POLICY` | `ask|none|analyze|fix`; default `ask` | Режим восстановления после сбоя. `analyze`/`fix` передают захваченный вывод провайдеру. |
| `--timeout=DURATION` | Go duration `0s`–`24h`; default `0s` (без timeout попытки) | Ограничить время дочернего процесса. |
| `--log-limit=BYTES` | integer `1024`–`16777216`; default `1048576` (1 MiB) | Максимальный общий захват stdout/stderr. |
| `--session=ID` | ключ из 10 символов; default пусто | Связать выполнение/восстановление с диалогом. |
| `--proxy=URL` | непустой URL; default пусто | Прокси HTTP(S) или SOCKS5 для восстановления/запросов, где поддерживается. |
| `--no-interactive` | boolean; `false` | Отключить интерактивные запросы восстановления. |
| `-h, --help` | — | Показать отдельную справку команды run. |

Коротких алиасов этих run-опций нет. `--help` обрабатывается отдельным парсером команды. Эти опции относятся к literal argv, а не к обычным Cobra-флагам. (В старых сборках могут отображаться параметры повторов; в описанном поведении одной попытки они удалены.)

## Скрипты: `zev script`

Управление сохранёнными Bash-скриптами. У родителя нет позиционных аргументов; локально help, наследуются proxy и verbosity.

### `zev script create [name [description [arguments-json]]]`

Создать скрипт; от 0 до 3 позиционных значений. Без переданного тела открывается настроенный редактор. Для неинтерактивного режима задайте `--body-file PATH` или `--body-stdin`.

**Локальные флаги:** `--argument NAME[=DESCRIPTION]` (string array, повторяемый); `--arguments-json JSON` (string, пусто; заменить порядок аргументов); `--body-file PATH` (string, пусто); `--body-stdin` (boolean, `false`; читать тело только из stdin); `--clear-arguments`, `--clear-description` (boolean, `false`); `--description TEXT` (string, пусто); `--edit-body` (boolean, `false`; явно открыть редактор, требуется TTY); `--generate-arguments`, `--generate-description` (boolean, `false`; разрешить AI-генерацию); `--json` (boolean, `false`; schema-versioned output без prompts); `--non-interactive` (boolean, `false`); help. Наследуемые: proxy и verbosity. `--body-file` конфликтует с `--body-stdin`; `--edit-body` конфликтует с noninteractive/передачей тела.

### `zev script edit [name]`

Изменить скрипт; имя необязательно, 0–1 аргументов. Те же локальные поля и значения, что у `script create`, за исключением отсутствующего `--no-metadata`; действуют флаги тела и генерации metadata. Наследуются proxy и verbosity.

### `zev script list`

Показать скрипты; аргументов нет. **Локальные:** `--json` (boolean, `false`), `--non-interactive` (boolean, `false`), help. **Наследуемые:** proxy и verbosity.

### `zev script remove [name]` (`rm`)

Удалить скрипт по необязательному имени; без имени — интерактивный выбор. **Алиасы:** `rm`, `delete`. **Локальные:** `--json` (boolean, `false`), `--non-interactive` (boolean, `false`), `--yes` (boolean, `false`; подтвердить удаление по имени), help. **Наследуемые:** `--proxy URL` (по умолчанию пусто), `-v, --verbose` (счётчик, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev script show <name>`

Напечатать тело одного скрипта; требуется ровно одно имя. Локально только help. Наследуются proxy и verbosity.

## Настройка: `zev setup`

Интерактивная первоначальная настройка провайдера. У родителя только help и наследуемые proxy/verbosity.

- `zev setup headers`: интерактивно заменить пользовательские HTTP-заголовки; аргументов и локальных флагов, кроме help, нет. Вводите по одному `Header-Name:Header-Value` на строку; пустая строка завершает ввод. Наследуются proxy и verbosity.
- `zev setup mode [inject|execute]`: ноль или одно значение; без значения показывает текущий режим, с ним — задаёт. Допустимо `inject` или `execute`; умолчание `inject`. Локально только help. Наследуются proxy и verbosity.

## `zev update [stable|main]`

Обновить stable-релиз (по умолчанию) или последний commit `main`, затем выполнить необходимые миграции. Допускается ноль или один positional branch, только `stable` или `main`.

До dispatch CLI Zev синхронно выполняет проверку обновлений в режиме best effort с таймаутом 5 секунд. Ошибки и таймауты не мешают запуску запрошенной команды. Неудачные или завершившиеся по таймауту проверки кешируются на 1 час до повтора; успешные результаты кешируются на 24 часа в `~/.zev/update-check.json`. Явная операция обновления имеет таймаут 1 минута для сетевой загрузки и установки.

**Видимые флаги:** help. **Наследуемые:** `--proxy URL` (пусто), `-v, --verbose` (счётчик, `0`).

**Скрытый внутренний флаг:** `--migrate-only` (boolean, `false`; скрыт в обычной справке). Используется только механизмом update/migration для запуска локальных миграций без скачивания и установки новой сборки; это не пользовательский режим обновления. Для локальных миграций он сохраняет отдельный таймаут 10 минут.

## `zev version`

Напечатать версию Zev; позиционных аргументов нет. Видимый локальный флаг: help. Наследуются proxy и verbosity.

**Скрытые внутренние probes:** `--update-protocol` (boolean, `false`; печатает маркер протокола обновления) и `--update-build` (boolean, `false`; печатает только build version). Их использует updater для проверки совместимости и определения сборки; это не пользовательские варианты команды version.

## Справка и completion

Справка корневой команды: `zev --help`; для команды: `zev COMMAND --help`. Справка алиаса/скрипта обрабатывается dispatch Zev: показывает локальное использование сущности, не отправляя запрос LLM. Команда `zev help` удалена; поддерживаются `--help`/`-h`. См. [Безопасность](SECURITY.md), [Алиасы и скрипты](ALIASES-SCRIPTS.md) и [руководство `zev run`](RUN-WRAPPER.md) для эксплуатационных границ.

Shell completion создаётся командами `zev completion bash|fish|powershell|zsh`; платформенные инструкции установки выводятся в справке соответствующей команды. Генерация не меняет shell-конфигурацию автоматически.

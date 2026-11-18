<div align="center">

[English](../en/ALIASES-SCRIPTS-SPEC.md) | **Русский**

</div>

# Спецификация: управляемые алиасы и скрипты

**Статус:** draft, редакция после adversarial review

## 1. Цель и границы

Zev должен дать единый предсказуемый механизм хранения и запуска алиасов и Bash-скриптов.
У каждой сущности могут быть описание и упорядоченный контракт позиционных аргументов.
Эти данные должны использоваться из одной validated model в списках, TUI, справке,
ошибках запуска и machine mode.

В scope входят:

- необязательные `description` и `arguments` для алиасов и скриптов;
- интерактивный и полностью командный CRUD;
- явная генерация недостающих метаданных через отдельный LLM-порт;
- `zev <name> --help` без выполнения и сетевых запросов;
- проверка обязательных аргументов до запуска;
- сохранение исходных границ `argv`;
- канонический YAML, строгая валидация и миграция старого формата;
- восстановление после сбоев публикации и защита от stale edit.

Не входят изменение правил `inject`/`execute` обычных query, генерация тела скрипта,
sandbox shell и удалённая синхронизация между компьютерами.

## 2. Capability map и порядок поставки

Это несколько независимо проверяемых возможностей. Они поставляются вертикальными срезами:

| Срез | Результат | Зависит от |
|---|---|---|
| `metadata-model` | общая модель, codec, validator, version-2 read/write | — |
| `alias-crud` | alias CRUD, help, единое argv-safe execution | `metadata-model` |
| `script-crud` | script metadata/body CRUD, help, exact argv | `metadata-model` |
| `metadata-generation` | отдельный LLM-порт, consent, fallback, provider adapters | `metadata-model`, CRUD |
| `migration-recovery` | legacy migration, archive, journal/recovery, concurrency tests | `metadata-model`, CRUD |
| `native-acceptance` | runtime-проверки shell/Bash на Linux, macOS и Windows | все предыдущие |

Каждый срез имеет собственные focused tests и checkpoint. Полный acceptance gate не
считается зелёным, пока не проверены все срезы; зелёный host test не заменяет native runtime.

## 3. Термины и модель

**MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT** и **MAY** имеют смысл RFC 2119.

- **Алиас** — именованный шаблон shell-команды.
- **Скрипт** — зарегистрированный Bash-файл и его metadata.
- **Metadata** — `description` и ordered `arguments`.
- **Позиционный аргумент** — один элемент исходного `argv`, переданный после имени.
- **Legacy source** — старый файл aliases/scripts до миграции.
- **Argv alias** — каждый канонический alias после миграции, использующий ограниченный
  template dialect и точную подстановку данных.

### 3.1. Общие metadata

```yaml
arguments:
  - name: service
    description: Имя или ID сервиса
    required: true
```

- `name` обязателен, уникален внутри одной сущности, ASCII-идентификатор длиной 1–64;
- `description` необязателен и может быть пустым;
- `required` по умолчанию `true`; отсутствие поля не должно уничтожать явное `false`;
- позиции идут с 1 и не могут быть разреженными;
- обязательные позиции должны идти перед необязательными;
- duplicate names, null, неверные типы, invalid UTF-8 и лишние поля отклоняются;
- metadata не содержит тело скрипта и не является вторым источником его содержимого.

Для новых записей имя соответствует `[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}` и сравнивается
с учётом регистра. Имена встроенных Cobra-команд (`alias`, `script`, `query`, `history`,
`setup`, `doctor`, `version`, `continue`, `help` и будущие зарезервированные имена)
запрещены. Алиасы и скрипты используют отдельные namespaces; при одинаковом имени
корневой вызов выбирает alias, что сохраняет текущий precedence. Явные `zev script ...`
и `zev alias ...` остаются однозначными.

Legacy-записи не переименовываются молча. Невалидное для нового CLI имя блокирует активацию
и остаётся неизменным в legacy source. Диагностика называет его и instructs пользователя
остановить writers Zev, сделать backup, вручную исправить legacy key и повторить миграцию.
Обычный CRUD не может ремонтировать заблокированную миграцию. Опциональная будущая команда
`zev alias repair rename` / `zev script repair rename` может выполнять такую операцию только
как offline locked repair; она не входит в первую реализацию.

### 3.2. Алиас

Алиас имеет `name`, `template`, `description` и `arguments`. Все алиасы используют единый
argv-safe контракт выполнения. Поля `mode`, флага выбора альтернативного режима, альтернативного renderer
и legacy-пути выполнения нет. После миграции старые и новые записи имеют одну модель.

Миграция компилирует каждый старый template в argv-safe dialect. Она конвертирует
поддерживаемые numbered placeholders, превращает старый implicit append в явный
`{{.args}}` и проверяет результат по закрытой grammar. Миграция никогда не сохраняет raw
legacy shell semantics. Старый template, который нельзя преобразовать без изменения границ
аргументов, блокирует миграцию с понятной диагностикой; ему не назначается legacy mode и
он не переписывается молча.

В argv-safe dialect поддерживаются только standalone placeholders:

- `{{.arg1}}`, `{{.arg2}}`, … — один shell word;
- `{{.args}}` — ноль или больше shell words в исходном порядке.

Диалект `argv` — закрытая grammar: одна простая команда из literal unquoted words и
standalone placeholders. В нём нет pipes, redirections, comments, assignments, quoting
вокруг placeholders, escaped newlines, command/arithmetic substitution, heredocs,
grouping, control operators и другого shell-синтаксиса. Placeholder внутри quoted или
concatenated word (`prefix{{.arg1}}`) отклоняется до сохранения и запуска. Интерпретатором
является настроенный POSIX-compatible shell; unsupported shell даёт явную ошибку. Значения
рендерятся алгоритмом POSIX single-quoted word. Fallback на более широкую template grammar
отсутствует.

В argv-safe dialect один вычислимый контракт аргументов:

- numbered placeholders MUST образовывать contiguous range `{{.arg1}}`…`{{.argK}}`, где
  `K` от 1 до 32;
- metadata объединяется по позиции; незадекларированная numbered position получает
  `required: true` и display name `argN`;
- metadata вне `1..K` отклоняется; `required: false` делает numbered position optional;
- required positions идут перед optional, пустое переданное значение считается присутствующим;
- каждый numbered placeholder выдаёт ноль или одно слово; пропущенный optional выдаёт ноль;
- `{{.args}}` выдаёт все переданные argv, включая аргументы, отдельно использованные
  numbered placeholders, и не задаёт positional minimum;
- без `{{.args}}` значения после `K` отклоняются до запуска shell;
- template без placeholders MUST иметь пустое arguments metadata и не принимает argv.

`{{.args}}` нельзя смешивать с implicit append. Requiredness определяется только этим
metadata/placeholder contract, а не эвристикой по произвольному Go template.

Все канонические aliases, включая migrated aliases и aliases из `alias last`, используют
argv-safe dialect. Help никогда не показывает legacy warning. Существующий shell-синтаксис вне
закрытой grammar является migration error: source нужно вручную исправить и повторить
миграцию; escape hatch в data model отсутствует.

### 3.3. Скрипт

Скрипт имеет `name`, immutable `recordID`, `script` (relative path inside `script_files`),
`description`, `arguments`. `recordID` генерируется при create и migration и обязателен в
version 2; его ручное удаление или изменение — conflict. Body не хранится в YAML: YAML владеет только metadata и validated path,
отдельный regular file владеет body. Execution и generation используют уже открытый
проверенный file descriptor и не переоткрывают заменённый pathname.

Скрипты не анализируются для вывода аргументного контракта. `arguments` — единственный
источник required semantics; body получает exact `argv` и `$@`. Без metadata скрипт
принимает любое количество аргументов, но help не обещает именованных параметров.
Существующие rooted path, no-symlink, regular-file, single-hard-link и pinned-file
проверки сохраняются.

## 4. Канонический YAML и строгий codec

### 4.1. Version 2

`~/.zev/aliases`:

```yaml
version: 2
aliases:
  dps:
    template: docker service ps {{.arg1}} --no-trunc
    description: Показать задачи сервиса Docker Swarm без сокращения вывода
    arguments:
      - name: service
        description: Имя или ID сервиса
        required: true
```

`~/.zev/scripts`:

```yaml
version: 2
scripts:
  deploy:
    recordID: 550e8400-e29b-41d4-a716-446655440000
    script: deploy.sh
    description: Деплой приложения в продакшен
    arguments:
      - name: environment
        description: Целевое окружение
        required: true
```

Scalar integer top-level `version` означает envelope: `version: 2` требует соответствующий
map `aliases`/`scripts`, а incomplete/invalid envelope даёт ошибку. Legacy map с entity
name `version` принимается только если значение имеет legacy record shape; при ambiguous
shape классификация завершается с ошибкой. Duplicate YAML keys, несколько документов, null
root, неверные типы и unknown fields дают ошибку; malformed v2 не fallback'ится в legacy.
Unknown newer versions не downgrading и не заменяются пустым файлом.

Общий persistence-neutral metadata model и semantic validator живут в нейтральном
internal package. Один versioned codec обслуживает оба repository. Alias/script
repositories владеют только path, lock и publication; CLI не разбирает YAML.

Валидация ограничивает также размеры: metadata files и archives максимум 1 MiB, YAML depth
не более 16 и 10 000 nodes, templates максимум 16 KiB, script paths максимум 512 bytes.
Name ограничен 64 bytes, description — 4096, argument name — 64, argument description —
1024, максимум 32 arguments. Decoder читает через bounded reader и отклоняет anchors,
aliases, merge keys, custom tags, NUL, C0/C1, ANSI escapes, invalid UTF-8 и trailing
documents. Terminal renderer обязан escape LF, backspace, ESC и другие display-affecting
controls; machine output JSON-encode каждый user field. Canonical YAML codec отклоняет
anchors, aliases, merge keys и custom tags.

### 4.2. Legacy migration и recovery

Legacy formats:

- aliases: YAML map `name: string-template`;
- scripts: YAML map с `script` и существующим `description`.

Migration обязана сохранять все существующие script descriptions byte-for-byte после
проверки UTF-8 и не менять body. Отсутствующие поля становятся пустыми; существующие
не заменяются пустыми. Каждый migrated alias компилируется и сохраняется как argv-safe dialect;
legacy mode не назначается ни одной записи.

Мигратор запускается перед первой операцией, требующей хранилище: list, show/help,
CRUD и named execution. `zev --help`, `version` и query, завершившийся до named lookup,
не создают metadata files. Если active file отсутствует, считается пустое хранилище;
если он read-only или malformed, операция завершается без записи.

После успешной миграции записи остаются пригодными без AI-обогащения. Одноразовое
post-migration enrichment, durable offer state и bulk consent перенесены в следующий
вертикальный срез; этот релиз их не заявляет.
Предложение об обогащении в этом релизе не показывается, не сохраняется и не выполняется.

Каждый migrator использует ровно тот же canonical lock identity и lock order, что и
соответствующий repository. Гарантии распространяются на cooperating writers; произвольная
запись в обход lock не входит в threat model.

Migration marker сам durable и private. До создания archive data нужно сохранить operation
ID, source fingerprint, deterministic staging/final archive paths и phase `preparing`; затем
write/sync owned staging archive, проверить полный hash, publish final archive без overwrite,
sync directory и перевести marker в `archived`. Только после этого повторно проверить source
identity/size/mtime/hash, atomic replace active file canonical v2, sync file/directory и
перевести marker в `published`. Marker updates используют same-directory temp files,
restrictive permissions, fsync и directory sync.

Recovery сверяет фактические source/archive/active hashes с marker. Неполный owned staging
можно удалить и создать заново из неизменённого source; matching published archive можно
adopt; occupied archive с другим hash или missing/corrupt marker блокирует операцию. Archive
никогда не перезаписывается. Любая ошибка sync/rename/marker после side effect
классифицируется как committed-but-durability-unknown (`errors.Is(..., filestore.ErrCommitted)`),
если publication могла произойти. Migration никогда не импортирует дважды.

Original source/archive никогда не удаляются автоматически. Archives, journals, markers и
staged bodies используют private directories, 0600-equivalent files/owner-only ACLs,
exclusive no-follow regular-file creation и не пишут payloads в logs. Cleanup failure
сообщается без удаления или overwrite неизвестного path. Occupied archive с другим hash —
конфликт. Corrupt/changed source, invalid legacy record и archive mismatch блокируют
активацию и сохраняют предыдущий активный файл.

### 4.3. Script body + metadata transactions

Операция, меняющая body и metadata, выполняется под script-store lock через staging и
recoverable journal с фазами `prepared`, `body-published`, `metadata-published`, `done`.
Journal хранит operation kind, immutable record ID, old/new metadata hashes, old/new body
hashes, path identities, staged/backup paths и checksum-protected phase. Перед любым чтением
или записью script store — включая list, help, show, generation и execution — процесс
получает тот же lock и восстанавливает pending journal. Recovery обязано оставить либо
полную old pair, либо полную new pair; unresolved mismatch блокирует доступ и execution
storage error. На платформах без этого протокола combined publication недоступна.

Script record имеет immutable generated `recordID`, сохраняемый при edits и назначаемый при
migration. Stale edit сравнивает его, selected metadata bytes, script-path identity и body
content hash/identity. Delete/recreate — conflict; unrelated record changes не отменяют
field-level edit. Ручное удаление или изменение `recordID` — conflict.

## 5. CLI-контракт и mode matrix

### 5.1. Алиасы

```text
zev alias add [name template [description [arguments-json]]]
zev alias edit [name]
zev alias remove [name]
zev alias list [--json]
zev alias last <name>
zev <alias> [argv...]
zev <alias> --help
zev <alias> -- --help
```

Flags:

```text
--template TEXT
--description TEXT
--argument NAME[=DESCRIPTION]   # replacement ordered list, required=true
--arguments-json JSON           # replacement ordered list, supports required=false
--clear-description
--clear-arguments
--generate-description
--generate-arguments
--no-metadata
--json
--non-interactive
```

Positional arguments and metadata flags mutually exclusive for the same field.
`--argument` и `--arguments-json` тоже mutually exclusive. Value/clear/generate для
одного field mutually exclusive. Edit — patch: отсутствующий field сохраняется,
переданный заменяет, `--clear-*` очищает. `--no-metadata` действует только на alias-add: пропускает metadata prompts/generation и
сохраняет пустые metadata; конфликтует с metadata values и отклоняется на edit. `--yes` для
именованного remove принимается как idempotent compatibility flag без дополнительного
эффекта; он не выбирает unnamed record и не bypass'ит interactive confirmation.

`zev alias add` без аргументов открывает interactive form. `zev alias edit` без name
показывает список всех алиасов и после выбора проходит по template, description и
arguments. Каждое поле предварительно заполнено; Enter сохраняет его, новый ввод заменяет.
`zev alias remove` без name показывает список и требует отдельного подтверждения.

### 5.2. Скрипты

```text
zev script create [name [description [arguments-json]]]
zev script edit [name]
zev script remove [name]
zev script list [--json]
zev script show <name>
zev <script> [argv...]
zev <script> --help
```

Дополнительные flags:

```text
--description TEXT
--argument NAME[=DESCRIPTION]
--arguments-json JSON
--body-file PATH
--body-stdin
--edit-body
--clear-description
--clear-arguments
--generate-description
--generate-arguments
--json
--yes
--non-interactive
```

`--body-file` и `--body-stdin` mutually exclusive. `--body-stdin` полностью владеет stdin
и означает non-interactive mode. `--json` также означает non-interactive для всех management
commands и никогда не открывает prompts, TUI или editor. Metadata-only edit не открывает
editor. `--edit-body` явно выбирает interactive editor path, требует TTY и конфликтует с
`--json`, `--non-interactive`, `--body-file`, `--body-stdin`. Конфликты дают code 2 до чтения
stdin и изменения storage.

### 5.3. Matrix выбора режима

| Вызов | TTY | Не TTY/explicit non-interactive |
|---|---|---|
| `alias add` без args | interactive form | usage error, no prompt |
| `alias add name template` | command mode, missing metadata остаётся empty | command mode |
| `alias edit` без name | list → select → form | usage error |
| `alias edit name` без edit flags | form with prefilled values | usage error |
| `alias edit name --field ...` | command mode, no TUI | command mode |
| `alias remove` без name | list → Yes/No | usage error |
| `alias remove name` | command mode, no confirm | command mode |
| `script create` без args | form + body editor | usage error |
| `script create name` без body source | existing editor journey | error before registration |
| `script edit name` без field flags | metadata form + optional body editor | usage error |
| `script remove` без name | list → Yes/No | usage error |
| `script remove name` | command mode, no confirm | command mode |

Interactive edit показывает текущие значения напрямую; Enter сохраняет. Interactive remove
подтверждает выбранную запись. `--yes` — только command-mode bypass. Cancellation/EOF до регистрации ничего не записывает. После commit default script и
metadata ошибка или cancellation editor оставляет committed skeleton и возвращает diagnostic;
это сохраняет текущее поведение. Staged-editor change будет отдельным compatibility decision.

Named command с complete fields никогда не открывает TUI/editor. Missing command-mode
fields проверяются до чтения body stdin и публикации.

### 5.4. Root dispatch, help и machine mode

Named dispatch получает исходный process `[]string` до любого join в natural-language prompt.
Только после not-found обоих stores query может выполнить `strings.Join(args, " ")`.
`strings.Fields` и эквивалентные tokenizers запрещены в named dispatch. Interactive free-form
query остаётся текстом, а не заявленным argv.

Built-in commands имеют precedence над entity lookup. Для найденной entity tail opaque,
кроме правил ниже: query flags, context files, history и provider setup не применяются.
Root machine flags должны находиться до entity name. `--` удаляет только первый delimiter и
передаёт все следующие argv verbatim, включая дальнейшие `--`.

Для найденной entity:

- до первого delimiter одиночный `-h` или `--help` запрашивает entity help;
- до первого delimiter любой help token вместе с другими tail arguments — usage error;
- после удаления первого delimiter все tokens literal, включая `-h`, `--help` и следующие delimiters;
- `name -- --help` передаёт один literal `--help`, а `name -- value --help` — два literal arguments.

Help показывает type, description, argv-safe dialect, template/file, таблицу arguments, usage и example.
Help не требует executable body и не вызывает LLM. Для существующей entity exit code 0,
для отсутствующей/невалидной usage — 2.

Сохраняется existing machine behavior:

- `zev --print <alias> [args]` валидирует/разворачивает alias и не выполняет;
- `zev --json <alias> [args]` возвращает существующий versioned query JSON и не выполняет;
- `zev --print/--json <script>` возвращает status 2 с пустым stdout; для body используется `script show`;
- `<entity> --print` и `<entity> --json` после name — literal entity argv.

Management JSON использует schema version 1. Успешный list: `{"schemaVersion":1,"status":"ok","type":"alias","items":[...]}` для aliases или
`type:"script"` для scripts; успешные add/create/edit/remove используют
`{"schemaVersion":1,"status":"ok","type":"alias","item":{...}}` (или `script`), remove возвращает removed record. Records содержат явные boolean `required`, а для alias — `template`, для script — `script`. `--json` non-interactive. Data и help идут в stdout, prompts/warnings/diagnostics — в stderr; failures дают пустой stdout. Exit codes: 0 success, 2 usage/validation/missing argument, 1 definite storage/launch failure, 6 `committed_unknown` с operation ID и reconciliation instructions, либо child exit code после запуска. Existing ordinary query machine codes 3/4/5 не меняются. Automatic retry для code 6 запрещён.

### 5.5. Missing arguments

Validation происходит до запуска shell/Bash:

```text
alias "dps" requires argument 1: service — Имя или ID сервиса
usage: zev dps <service>
help: zev dps --help
```

Сообщение называет type, entity, position, name и description. Empty argv value считается
переданным. Все aliases без `{{.args}}` отклоняют значения после `K`. Script metadata задаёт
только minimum required positions; extra script values передаются `$@`.

## 6. Argv boundaries и execution

Invoking shell удаляет внешние кавычки до старта Zev:

```bash
zev dps "service with spaces"
zev dps 'service with spaces'
```

Оба вызова дают один argv element. Zev не должен повторно его разбивать. Тесты покрывают
empty strings, tabs, newlines, Unicode, quotes, `$`, `*`, `;`, `&&`, `$()` и backticks.

В argv-safe dialect numbered placeholders рендерятся в ноль или одно shell word по правилам
presence/requiredness, а `{{.args}}` — в ноль или больше independently quoted words из
original argv. Unsupported contexts отклоняются; generic string replacement не
рекламируется как safe. Новые и мигрированные aliases используют одинаковые execution
rules; отдельного renderer или compatibility execution mode нет.

Scripts передают exact argv в Bash без join/reparse. Runtime matrix:

| OS | Alias | Script | Acceptance |
|---|---|---|---|
| Linux/macOS | POSIX-compatible `$SHELL` | Bash в PATH, `/dev/fd` Unix path | native runtime tests |
| Windows | explicit allowlisted compatible shell | handle-safe body transport; no pathname fallback; иначе explicit unsupported error | native Windows gate |

На Windows implementation обязан определить handle/descriptor transport, argv encoding,
reparse-point/hard-link checks и shell allowlisting; fallback на reopened pathname,
PowerShell или `cmd.exe` запрещён. Cross-compilation не доказывает runtime. Child stdout/stderr
остаются child output; Zev
добавляет собственный текст только в свои streams. Signal и non-zero exit propagation
проверяются реальным built binary.

## 7. Metadata generation через LLM

Generation использует отдельный consumer-side port, не query `InferenceProvider`:

```go
type MetadataGenerator interface {
    GenerateDescription(ctx context.Context, input DescriptionInput) (string, error)
    GenerateArguments(ctx context.Context, input ArgumentsInput) ([]Argument, error)
}
```

Request содержит только entity name, alias template или bounded pinned script body и уже
заданные metadata. В нём нет history, ordinary system prompt, environment context, tools,
API keys или unrelated files. Provider adapter использует dedicated metadata prompt и
валидирует response schema.

Interactive generation сообщает, что полный serialized request — entity name, existing
metadata, template/body, destination, remote/local status и byte size — может быть отправлен
на настроенный endpoint. Explicit confirmation разрешает эту передачу на один invocation. В
command mode соответствующий `--generate-*` является consent; без него network generation
запрещён. Ни один механизм не создаёт persisted consent. Local-only policy MAY запрещать
remote generation и возвращает понятную ошибку.

Общие limits и validator применяются к ручному и generated input. Generation request,
включая pinned script body или template, ограничен 65 536 UTF-8 bytes и не truncates silently.
Provider response body ограничен 65 536 bytes до decode; допускается один JSON document,
bounded nesting, максимум 32 arguments и отсутствие trailing data. Invalid UTF-8, controls/
ANSI, duplicate argument names, invalid booleans, unknown fields и oversized values
отклоняются. На add failure оставляет поле empty и сохраняет entity. На edit failure
сохраняет старое поле. Warning идёт в stderr; invalid candidate не считается успехом.
Generation запрещена внутри мигратора и обычных help/list/remove/execution paths.
Однократное обогащение после миграции из §4.2 — отдельный явно разрешённый шаг, а не часть
преобразования данных; его ошибки не могут сделать миграцию неуспешной.

## 8. Drift, locking и manual edits

Canonical YAML — единственный persistent source of truth. Нет отдельных help/argument/AI
registries. Все consumers reread через repository и один validator.

Каждое изменение перечитывает active file под exact repository lock, валидирует полный набор,
меняет только выбранную запись и публикует atomically. Malformed data, unknown version,
duplicate keys или conflict оставляют предыдущий active file без изменений.

List/help видят manual edit на следующем invocation. Каждый script-store reader сначала
делает journal recovery под repository lock; unresolved journal возвращает storage error и
никогда не показывает или выполняет mixed state. Cooperation гарантирована только для
writers, использующих тот же lock; direct unlocked writes вне протокола. Cache живёт только
внутри invocation.

## 9. Compatibility

Старые команды сохраняются: `alias rm`, `script rm`, `alias last`, `script show`, existing
editor selection и alias precedence. `alias last` использует то же argv-safe представление,
что add и migration, сохраняет пустые metadata, не вызывает LLM и не перезаписывает имя.
Команды вне поддерживаемой template grammar отклоняются без создания alias. Legacy
exception отсутствует.

Migration меняет storage, добавляет metadata и компилирует старые alias templates в argv-safe
syntax; unsupported raw shell semantics не сохраняются. Эта release отклоняет schema
versions newer than supported без изменения файлов. Запуск
pre-v2 binaries с migrated stores unsupported; новая release не может контролировать их
поведение. Перед downgrade нужно остановить writers, сохранить v2 stores и явно восстановить
выбранный legacy archive; изменения после archive при этом не сохраняются.

## 10. Testing и acceptance

### Unit

- strict codec round-trip для entities, omitted/true/false `required`, null/wrong types,
  duplicate keys, unknown fields, multiple documents и legacy names `version`/`aliases`;
- validator limits, controls, UTF-8, names, collisions, sparse/optional argument rules;
- argv-safe template parser и rejection quoted/embedded/unsupported placeholders;
- exact argv substitution для spaces, empty values, quotes, shell metacharacters, Unicode/LF;
- missing/extra argument diagnostics и help rendering;
- metadata generator request/result validation и per-field fallback;
- stale metadata/body snapshot conflicts.

### Integration/e2e

- real root dispatch с aliases, scripts, built-ins, corrupt stores и invalid provider config;
- `zev dps "a b"`, `zev dps -- --help`, `zev dps value --help` и repeated `--`;
- root `--print/--json` никогда не выполняет, entity-tail flags остаются literal;
- real child captures exact argv и marker file не создаётся metacharacters;
- interactive/non-TTY matrix, EOF/cancel, no partial writes, editor failure compatibility;
- management JSON schema, stdout/stderr и exit status 0/1/2/7/127;
- recording local provider доказывает consent, bounded body, отсутствие history/tools/config leakage;
- legacy migration сохраняет descriptions, bodies, bytes, archive hash и retries;
- crash injection before/after archive, body publication, metadata publication, rename, sync/cleanup;
- concurrent cooperating CRUD/migration, stale edit и same-name alias/script collision;
- native Linux/macOS/Windows runtime gates, missing Bash и signal cleanup.

### Completion

Feature complete только когда каждый capability-map slice проходит checkpoint, все acceptance
tests зелёные, `make test`, `make test-race`, `make lint` с post-lint diff inspection, `make check` и
`make build` проходят. Так как `make lint` в этом repository может менять files, implementation
owner обязан проверить и сообщить resulting diff; native gaps явно перечислены и не выводятся
из cross-compilation.

## 11. Граница реализации

Политика миграции определена: все старые записи переходят на новое представление;
legacy-режима выполнения нет. После успешной миграции одно сообщение показывает результат
и предлагает добавить недостающие описания и аргументы через ИИ. Отказ или ошибка
провайдера оставляет мигрированные записи рабочими без этих необязательных полей.

Других мастеров миграции, выбора режима и подтверждений обогащения для каждой записи нет.
Остальные разделы задают требования к реализации, а не дополнительные вопросы при старте.

Эта задача меняет только спецификацию. Реализация, установка зависимостей, миграция реальных
файлов пользователя и коммиты требуют отдельного запроса на реализацию.

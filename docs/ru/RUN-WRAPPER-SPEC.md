<div align="center">

[English](../en/RUN-WRAPPER-SPEC.md) | **Русский**

</div>

# Спецификация: самостоятельный zev run — выполнение, анализ и исправление ошибок

**Статус:** реализованы literal argv execution, ограниченный захват вывода, retry и интерактивное
восстановление после ошибки. Analyze и fix запрашивают варианты у provider только после выбора;
для injected-команды восстанавливается точный связанный диалог, а прямой запуск отправляет
самостоятельный запрос с контекстом ошибки. Обёртка query inject управляется параметром
`command_wrapper_enabled` (по умолчанию `true`). Предложения никогда не исполняются автоматически.
Журнал событий, сохранённые артефакты, часть расширенных флагов и независимое ревью остаются.

## 1. Цель и согласованные решения

У Zev два равноправных пользовательских входа:

| Вход | Задача пользователя | Результат |
|---|---|---|
| `zev query <запрос>` (или корневой запрос) | «Что выполнить?» | Команды по запросу на естественном языке |
| `zev run -- <команда> [args...]` | «Выполни мою команду; если упадёт — помоги разобраться» | Реальное выполнение, видимый лог и выбор анализа/исправления |

`run` — самостоятельная основная функция, а не только внутренний wrapper для query.
Пользователь может принести любую явно выбранную executable-команду; предшествующий
query, conversation ID и история LLM не нужны. Автооборачивание LLM-команд — дополнительная
интеграция этого же runner. Он не глобальный shell hook, не sandbox и не file-editing agent.

Обязательный пользовательский сценарий:

1. При ошибке **сначала всегда показывается лог**, затем разрешаются дальнейшие действия.
2. В интерактивном режиме четыре основных действия: **«Повторить команду»**,
   **«Проанализировать ошибку»**, **«Исправить команду»**, **«Ничего не делать»**.
   Начальный выбор — **«Ничего не делать»**.
3. Без интерактивности по умолчанию ничего не делать. Автоматические действия разрешаются
   только явными флагами текущего запуска.
4. «Исправить команду» открывает существующий **Clarify request**. Уточнение необязательно:
   пустой/whitespace-only Enter всё равно отправляет ошибку и лог в LLM. Escape/EOF/отмена
   не являются отправкой. Отдельный Analyze объясняет ошибку и даёт рекомендации,
   не запускает сценарий исправления и не требует Clarify request.
5. Default wrapper применяется **только к LLM-командам**. Aliases/scripts и query
   `--print`/`--json` не начинают автоматически выполняться через wrapper. Это ограничение
   автоинтеграции: явный `run` доступен независимо от происхождения команды.
6. Имена всех встроенных корневых команд и их синонимы запрещены для новых алиасов.
7. Согласованные defaults: `original` exit policy, лог 1 MiB на попытку, retention
   7 суток / 100 MiB. Исполнение исправления требует отдельного согласия.
8. Обычный `zev continue` не прикрепляет лог автоматически: это делает пользователь.
   Внутренний wrapper handoff не меняет этот ручной контракт.

Нормативные MUST/MUST NOT обозначают проверяемые требования. Инженерные уточнения ниже
не разрешают реализацию, новые зависимости, миграции, публикацию или коммит.

## 2. Исходное состояние и границы

Проверенные точки интеграции:

- [query.go](../../internal/cli/query.go): разбор флагов, доставка LLM-команды, выбор и уточнение.
- [continue.go](../../internal/cli/continue.go): сейчас восстанавливает последний диалог,
  спрашивает лог и формирует refinement даже из стандартного fallback-текста. Этот flow
  нельзя вызвать неизменённым для run recovery.
- [terminal](../../pkg/terminal/inject.go): shell-текст выполняется через `$SHELL -c`, fallback `sh`.
- [childprocess](../../internal/core/childprocess/run.go): общее владение process group/job;
  существующий API сам по себе не предоставляет полноценную PTY-сессию и не ограничивает
  произвольные блокирующие Readers/Writers.
- [provider](../../internal/core/domain/provider.go): варианты команд, не доказательство исправления.
- [tool executor](../../internal/entities/tool/repository/executor.go): ограниченный read-only путь.
- [history](../../internal/entities/history/service/types.go): conversation/command IDs и один
  изменяемый execution summary, но не журнал независимых run attempts.
- [filestore](../../internal/core/filestore/filestore.go): атомарная замена одного файла;
  `ErrCommitted` означает видимый rename с неизвестной crash-durability.

В первую версию входят явный argv-runner, default wrapping, error UI/Clarify request,
ограниченные retry/logs, Analyze без исправления, привязанный к run внутренний handoff,
приватные artifacts и события. В README/help run представляется рядом с query, имеет
самостоятельные примеры/сценарии, а не прячется только в разделе интеграции query.
Не входят перехват всего shell, новые write-tools, автономное выполнение исправлений,
автоматическая AI/regex-классификация как триггер, поиск run, dashboard, новые SQLite-схемы
или снимок всего проекта. Runner наблюдает явно запущенную через него команду; он не читает
shell history, не сканирует старые терминальные сессии и не обещает восстановить их stderr.
Все runtime-контракты здесь целевые; наличие существующего пакета не доказывает их реализацию.

## 3. CLI и порядок разбора

### 3.1. Явный run

```bash
zev run -- go test ./...
zev run --on-failure=none -- go test ./...
zev run --on-failure=retry --max-retries=2 --retry-delay=2s -- go test ./...
zev run --no-interactive --on-failure=analyze -- go test ./...
zev run --no-interactive --on-failure=fix -- go test ./...
zev run --exit-code=last --on-failure=retry -- go test ./...
zev run -- sh -c 'go test ./... && go vet ./...'
```

`run` имеет отдельный parser, не проходит query parser. `--` обязателен для исполнения;
собственный `zev run --help` не требует разделителя и ничего не запускает. После разделителя exact argv передаётся буквально: без join,
`eval`, glob expansion или повторной интерпретации shell. Для shell-программы пользователь
явно запускает shell. `cd` не является executable в argv-режиме.

| Флаг run | Default | Контракт |
|---|---|---|
| `--on-failure=ask\|none\|retry\|analyze\|fix` | `ask` | Меню / без действий / retry / анализ без исправления / исправление через LLM |
| `--no-interactive` | `false` | Запретить все вопросы run/recovery |
| `--max-retries=N` | `1` | Число повторов исходной команды, 0–10; всего не больше N+1 попыток |
| `--retry-delay=D` | `0s` | Отменяемая задержка автоматического retry, 0–5 минут |
| `--exit-code=original\|last` | `original` | Код первой / последней фактически выполненной попытки исходной команды |
| `--log-limit=BYTES` | `1048576` | Суммарный encoded sanitized tail на попытку, 1024–16777216 байт |
| `--timeout=D` | `0s` | Deadline одной попытки; 0 не добавляет deadline |
| `--events-file=PATH` | не задан | Отдельный приватный JSONL metadata sink |
| `--session=ID` | не задан | Ровно 10-символьный ключ локального реестра для точного диалога и выбранной команды; не latest |
| `--no-history` | `false` | Не открывать SQLite, не создавать run artifacts/диалог |
| `--no-context` | `false` | Не добавлять автоматический environment/Git context в LLM |
| `--tools` / `--no-tools` | tools disabled | Явное разрешение / запрет recovery tools |
| `--tool-approval=manual\|auto` | `manual` | Политика только явно включённых recovery tools; bypass запрещён |

ID сессии — непрозрачный ключ длиной 10 символов, а не обратимое кодирование двух UUID. При inject точная пара UUID диалога и команды записывается в приватный `run-sessions.json` атомарно под блокировкой; recovery разрешает ключ и отклоняет отсутствующие или повреждённые записи. Это сохраняет точную адресацию без двух UUID в shell-строке и без принятия коллизии hash за другую сессию.

Валидация аргументов завершается до child launch и provider setup. Usage error даёт `2`:
пустая команда, отсутствие `--`, неизвестные pre-separator options, невалидные значения,
отрицательные duration, невалидный session token, конфликт tools/no-tools,
либо явный approval без tools. Верхний предел timeout — 24 часа. Синтаксис IDs проверяется
до запуска. Если effective history disabled, supplied session key игнорируется с предупреждением,
без SQLite и загрузки диалога; команда всё равно исполняется. Это единое правило для
явного и generated invocation. In-process связь с активной памятью допустима без SQLite.

`--on-failure=retry` разрешает повтор **всей** команды с возможными частично применёнными
побочными эффектами. `analyze` и `fix` разрешают раскрыть видимый очищенный diagnostic bundle
настроенному провайдеру, но не выполнить исправление. Help MUST объяснять оба последствия,
возможность terminal echo и отсутствие гарантии удаления любых секретов. Автономная
политика не включается сохранённым «всегда исправлять» config. Ранее предложенное имя
policy `agent` не поставлялось и не сохраняется как дополнительный alias: это usage error.

**Локальный запуск независим от LLM.** До явного Analyze/Fix runner не создаёт provider,
не проверяет API-ключ/модель/endpoint и сам не отправляет LLM/embedding-запросы. Отсутствие
или невалидность provider settings не блокируют successful/none/retry run. Загружается
только необходимая валидная runtime/privacy-конфигурация; её собственные ошибки не скрываются.
При недоступном provider после выбора AI-действия остаются команда, лог и прежний exit policy,
выводится безопасная ошибка. Собственные сетевые действия child этим не ограничиваются.

### 3.2. Query dispatch и совместимость аргументов

Порядок: встроенный routing → root-prefix options и named lookup на исходном argv →
opaque tail найденной entity → query parsing только после not-found aliases/scripts.
Сохраняется [контракт named dispatch](ALIASES-SCRIPTS-SPEC.md): после имени entity
`--wrap`, `--json`, `--no-history` и другие query options — literal argv, кроме существующих
правил entity help/первого `--`. Root machine flags действуют только до имени entity.
Явный root-prefix `--wrap`/`--no-wrap` перед найденной entity отклоняется как неприменимый,
а не меняет её выполнение. Ни lookup, ни help не вызывают LLM.

Новый dotenv-параметр `COMMAND_WRAPPER_ENABLED=true`; precedence:
`--wrap`/`--no-wrap` → environment → `~/.zev/config` → `true`. Флаги взаимоисключающие.
Machine query возвращает исходный command/JSON; явные wrap flags там — usage error.

```bash
zev 'запусти тесты'
zev --no-wrap 'запусти тесты'
zev --wrap --execute 'запусти тесты'
```

## 4. Default delivery и имена команд

### 4.1. Inject и execute

- `execute`: общий runner вызывается in-process с типизированными параметрами, без
  второго executable и повторного разбора shell-текста.
- `inject`: wrapper invocation использует **абсолютный путь текущего executable Zev**,
  а не bare `zev` из PATH. Путь, argv и исходный shell-текст quoting-уются по receiving
  dialect; весь исходный shell-текст остаётся одним аргументом `-c`.
- Успешный inject не печатает выбранную команду; fallback при отказе injection выдаёт тот же
  invocation и не выполняет его.
- Точная identity текущего бинарника означает выбранный executable path на момент доставки,
  не гарантию неизменности файла при более позднем ручном запуске. Подмена same-UID вне модели.
- Исполняющий interpreter выбирается по существующему правилу `$SHELL`, затем `sh`,
  разрешается до executable path и фиксируется для всех попыток. Wrapper не добавляет
  `pipefail`, `set -e`, login mode или startup options.
- **Receiving dialect** и interpreter — разные сущности. Receiving dialect auto определяется
  только по подтверждённым OS-данным вызывающего shell; `$SHELL` не является таким
  доказательством. Новый query-only `--receiving-shell=auto|posix` (default auto) позволяет
  явно заявить POSIX receiving dialect для wrapped inject/printed fallback.
  Поддерживаемая первая grammar — POSIX quoting; auto допускает проверенные sh/bash/dash/zsh.
  Неизвестный/неподдерживаемый receiving shell даёт ошибку до доставки; raw fallback запрещён.
  Флаг неприменим к machine query, entity и execute, там usage error. Это не выбор interpreter.
- `cd`, `export`, shell functions и job control внутри child не изменяют родительский shell;
  нужен явный `--no-wrap`. Setup/help MUST показывать это ограничение.

**Через границу inject обязательно переносятся эффективные ограничения.** Если query
отключил history/context флагом или config, invocation содержит соответствующие
`--no-history`/`--no-context` до разделителя. Позднейшая смена config не может снять эти
запреты. Если они были включены, более строгие новые настройки runner имеют приоритет.
IDs передаются только при доступной истории. При её последующем отключении применяется
единое правило §3.1: игнорировать IDs с предупреждением без SQLite, выполнять без связи.
Generated invocation с уже выключенной историей IDs не включает.

Текст истории остаётся исходной командой модели. History write failure не запрещает
первый запуск, но связь не выдумывается. При history-disabled inject первый query-процесс
не передаёт свой in-memory dialogue второму: восстановить его без отдельного транспорта
невозможно; Analyze/Fix начинает отдельный in-memory диагностический разговор.
Явное разрешение tools переносится только согласно §8.3; сохранённое разрешение не переносится.

### 4.2. Reserved names

Все встроенные **корневые** имена, их command aliases, скрытые и автоматически добавляемые
команды зарезервированы: например query/continue/history/setup/doctor/alias/script/version,
новая run, help и completion. Список не является источником истины. Сравнение соответствует
точному регистрозависимому routing; имена только вложенных подкоманд не резервируются.

Общий validator MUST действовать во всех create/save-from-history/generate/rename/edit-name
и loaded-data путях, в том числе при прямом вызове сервиса. Отказ до записи называет
конфликт и не меняет данные. Registry coverage test сверяет все фактические root-команды,
синонимы, hidden/help/completion, без импорта CLI в domain. Добавление команды обновляет
единый контракт, а не wrapper-only blacklist. Существующий конфликт не удаляется,
не переименовывается и не исполняется вместо builtin молча: нужна видимая диагностика
и инструкция по явному исправлению; автоматическая миграция не разрешена.

## 5. Outcomes, действия и обязательный показ лога

Eligible failure — ненулевой exit code нормально завершённого child либо достоверная
ошибка старта без запущенного процесса. Stderr при exit 0 — успех, не триггер recovery.
Timeout, parent cancellation, child-only signal, unknown supervision/capture/cleanup outcome
показывают диагностику, но **не открывают recovery** даже при retry/analyze/fix policy.

```text
validate → execute → finalize capture/cleanup → outcome
  success → finish
  cancelled/timed_out/signalled/unknown → show available log → finish
  eligible failure → show log barrier → resolve policy
    none → finish
    retry → next attempt → same outcome handling
    analyze → provider → analysis_ready → report → finish
    fix → optional Clarify request → provider → correction_ready → finish or consented correction
```

**Display barrier:** после окончания child/cleanup retained log фиксируется, затем wrapper
полностью передаёт его diagnostic sink (stderr/UI), и только после успешного write/render
может начать retry/provider call. Это означает передачу sink, не доказательство, что человек
прочёл текст. Artifact без показа лога недостаточен. Broken diagnostic sink запрещает
recovery и даёт `125`, даже при явном headless analyze/fix. Запись `failure_log_shown` следует
после barrier, не до него. При display failure используется `display_failed`, если events
sink ещё работает. При аварии возможен только best-effort показ уже доступной части лога.

Экран: маскированная команда, cwd, attempt number/duration/outcome, раскрытый прокручиваемый
лог с начальной позицией в конце, truncation и artifact path при наличии.
Пустой вывод явно обозначается вместе с ошибкой запуска/кодом.

```text
Команда завершилась с кодом 1 · попытка 1
$ go test ./...
--- лог ---
... вывод ...
--- конец лога ---
  [Повторить команду]
  [Проанализировать ошибку]
  [Исправить команду]
> [Ничего не делать]
```

| Policy | UI доступен | UI недоступен |
|---|---|---|
| ask | Лог, меню с default none | Лог, none без чтения ввода |
| none | Лог, завершение | Лог, завершение |
| retry | Лог, ограниченный retry | Лог, ограниченный retry |
| analyze | Лог, запрос анализа, отчёт без исправления | Лог, запрос анализа, отчёт без исправления |
| fix | Лог, Clarify request, запрос исправления | Лог, запрос исправления с пустым уточнением |

Enter меню с исходным выбором, Escape/EOF/закрытие/ошибка UI означают none и сохраняют
command-result policy. Отказ инициализации UI допускает headless explicit policy только
**до** начала взаимодействия. Если Clarify request уже открыт, его ошибка/отмена никогда
не превращается в автоматическую отправку. Ошибка UI отличается от ошибки display sink.

Ручной Retry в ask после нового failure возвращает меню с default none. Кнопка retry
disabled при исчерпанном budget. Автоматический retry завершается после budget без analyze/fix.
Retry повторяет snapshot argv, cwd, environment и I/O policy; filesystem/external state
не фиксируются, частичные side effects не откатываются. Следующий attempt запрещён до
подтверждённой cleanup предыдущего. AI handoff (analyze либо fix) — не больше одного на
invocation. Отчёт анализа не запускает Fix без нового явного пользовательского запроса.

## 6. Анализ без исправления и отдельный сценарий Fix

### 6.1. Проанализировать ошибку

Выбор **«Проанализировать ошибку»** после display barrier явно разрешает отправить показанный
sanitized bundle в LLM в режиме `analyze`. Дополнительный Clarify request не требуется:
пользователь получает объяснение ошибки и рекомендации без генерации исполняемого исправления.
`--on-failure=analyze` делает то же без меню, в том числе headless; default не меняется.

Ответ анализа — отдельный bounded тип `FailureAnalysis` версии 1: краткое объяснение,
вероятные причины с опорой на наблюдаемые строки/коды, рекомендации по проверке/устранению
и явные ограничения уверенности. Поля: `schema_version=1`, непустой `summary`,
`causes` (список объектов со строкой `reason` и списком строк `evidence`),
`recommendations` (список строк),
`uncertainty` (строка). При неизвестной причине causes может быть пустым, uncertainty
объясняет нехватку данных. Общий encoded JSON ≤32 KiB, текст — валидный UTF-8;
неизвестные/невалидные поля и переполнение отклоняются до вывода. HTTP body ограничен
существующими 256 KiB. Невалидный raw response не печатается вместо отчёта; вывод проходит
те же terminal escaping/redaction. В history сохраняются mode/outcome/reference, не тело
отчёта и не выдуманные command alternatives; текст анализа остаётся эфемерным. Это не OptionsResponse
с фиктивным invalid-request и не список исполняемых Command. При недостаточном логе модель
должна назвать неизвестное, а не выдавать предположение за установленную причину.

Рекомендации могут содержать примеры команд как **инертный текст**. Analyze никогда не
показывает selector исполнения исправлений, не вызывает Execute/Inject, не меняет файлы
и не переходит в Fix автоматически. Это относится и к ответу с command-like текстом,
ошибочно возвращёнными исправляющими объектами или hostile instructions. Host-side dispatch
фиксирует режим до provider call и не позволяет ответу модели поменять его.

После validation отчёт печатается в stderr и invocation завершается с прежней command-result
policy. Successful analysis не превращает исходный failure в успех и не доказывает устранение
ошибки. Explicit read-only tools разрешены только по §8.3; это не право на модификацию.

### 6.2. Исправить команду

Fix получает типизированный bundle и optional clarification, а не общий empty-prompt
refinement. **«Исправить команду»** открывает существующий Clarify request. Успешный submit
пустого/whitespace текста валиден: provider получает стандартную инструкцию исправить команду
по ошибке. Ошибка/лог уже приложены; повторного paste-log/context-file questionnaire нет.
Escape/EOF/input error/cancel — не submit и не вызывают LLM. `--on-failure=fix` пропускает
меню; с UI Clarify сохраняется, без UI отправляется bundle с пустым уточнением.

В history blank clarification не создаёт refinement. Стандартная инструкция не выдаётся
за пользовательский текст: добавляется только при сборке provider request. Непустое уточнение
сохраняется как пользовательское. Для empty-log failure достаточно structured exit/start error;
не требуется выдумывать непустой log. Обычные query/refine/continue сохраняют свои правила.

После validated Fix response состояние — **correction_ready**: preview исправленных
вариантов и отдельный осознанный выбор исполнения конкретного варианта. До согласия запрещены
Execute, Inject и автоматический выбор первого варианта из configured execute mode.
Подтверждение показывает exact command и предупреждения до исполнения; default — отказ.
После согласия выбранное исправление выполняется отдельным supervised correction run с
`on-failure=none`, без автоматического retry/analyze/fix. Он имеет свой ID/outcome и не подменяет
исходный command-result policy. Отказ/отмена ничего не исполняют и не вставляют.
Без UI исправленная команда с пояснением печатается в stderr; execution/injection отсутствуют.
Исходная команда после исправления не перезапускается автоматически. Ответ модели — предложение,
не доказательство успешного исправления; проверка требует нового явного запуска.

### 6.3. Общие ограничения AI-обработки

Оба режима используют тот же failure bundle, redaction, trust boundary и tool policy.
Provider создаётся только после явного выбора/флага и успешного display barrier.
Ни analyze, ни fix не являются локальным распознаванием произвольного текста stderr.
Выбранный режим `analyze|fix` сохраняется в typed request, event metadata и исходном run
manifest; он не выводится из ответа, формулировки prompt или presence поля commands.

Весь provider/tool loop ограничен единым recovery deadline 120 секунд с момента его запуска.
Этот срок не включает ожидание пользовательского ввода; все пять adapters, HTTP retries
и tool turns получают один отменяемый bounded context, не сбрасывают deadline между turns.
Более строгие существующие tool/network timeouts сохраняются. Истечение recovery deadline —
provider failure: без исполнения исправления и без нового recovery, с сохранением
command-result policy. Parent SIGINT/SIGTERM и cleanup failure имеют приоритет по §10.
Owned сетевые операции и cooperative workers отменяются и завершаются; неотменяемые
внешние/kernel вызовы не выдаются за доказанно bounded и подчиняются ограничениям §7.2.

## 7. I/O, владение процессами и диагностические данные

### 7.1. Execution streams отдельно от UI

| stdin / stdout / stderr | Execution I/O |
|---|---|
| Все три TTY и один подтверждённый terminal session | PTY/platform equivalent; merged terminal stream допустим |
| Любой descriptor redirected, включая stdout pipe или только stderr redirect | Pipes/pass-through; stdout/stderr остаются раздельными byte streams |

Доступность управляющего терминала для UI — отдельная проверка. Diagnostic sink — stderr;
интерактивный TUI разрешён только если stderr является TTY и доступен отдельный
controlling-terminal handle для ввода. Иначе выводится plaintext log, взаимодействие
недоступно и применяется headless policy. Не читать redirected stdin/data stdout как UI.
`--no-interactive` запрещает вопросы. Wrapper не записывает отдельный stdin stream.

**Точность относится к пересылаемым байтам child, а не ко всему stderr descriptor.**
В pipe/pass-through child stdout/stderr не меняют байты/CRLF и не смешиваются между собой.
Stdout содержит только data child. Stderr дополнительно содержит явно отделённые
wrapper diagnostics и повторный failure log, в том числе при `2>file`; такой итоговый файл
не обещается byte-for-byte копией child stderr. Events туда не добавляются. Raw child
output отдельно в памяти/capture можно проверить независимо от replay diagnostics.
На PTY пути terminal processing/CRLF и объединение потоков допустимы и отражаются
`stream=terminal`, `stderr_present=unknown`. Live output не sanitizes-уется wrapper.
При capture/replay преобразуется только диагностическая копия.

Pipe/redirected stdin не воспроизводится: retry disabled; явный retry policy отклоняется
до child launch. Исключение — достоверный DEVNULL/заведомо пустой stdin без чтения данных
для «проверки». Не ждать EOF pipe ради определения пустоты.

### 7.2. Terminal session и cleanup

Terminal-session port владеет PTY master/slave, controlling session/foreground group,
resize forwarding, termios restore и закрытием handles в общем lifecycle с process owner.
Нельзя механически передать PTY slave в старый foreground-stdin алгоритм и объявить поддержку.
Требуются native fixtures до включения платформы: interactive input, resize, Ctrl-C,
mode restoration, descendant termination и echo. Фоновый job control/daemon detachment
не поддерживаются. Внешние brokers и отделившиеся Unix sessions вне ownership boundary.

Отдельный cleanup budget — 5 секунд на attempt/correction после cancellation/leader exit,
не отменённый request context. Нужны: лидер reaped, разрешённая платформой проверка
завершения управляемых descendants, закрытые PTY/pipe endpoints, завершившиеся capture workers,
восстановленный terminal. Send signal/успешный Kill syscall сами по себе не доказывают cleanup.
Неизвестность любого required evidence даёт cleanup_unknown/125, без нового launch/recovery.
Гарантия не распространяется на неотменяемый kernel I/O: нельзя обещать wall-clock bound
для зависшего filesystem syscall или устранение unkillable kernel task. Такие ограничения
и незавершённая cleanup явно сообщаются; ownership не считается освобождённым по timeout.
Не принимать произвольные блокирующие Reader/Writer callback как якобы ограниченные этим budget.

### 7.3. Capture и приватность

Диагностическая копия: bounded tail, best-effort redaction известных credential patterns и
настроенных секретов Zev, escaping terminal controls/invalid UTF-8. Encoded retained bytes
после escaping/redaction не превышают log-limit; raw discarded-byte count хранится отдельно.
Порядок между независимыми stdout/stderr — наблюдаемый capture order, не глобальный causal order.
Redaction работает через chunk boundaries с ограниченным lookbehind; нельзя гарантировать
распознавание любых секретов. Raw copy отдельно на диск не сохраняется.

**PTY echo:** отсутствие отдельного stdin capture не означает отсутствия введённых данных
в terminal output. У PTY artifact есть `echo_possible=true`; видимый лог и Clarify request
предупреждают, что он может содержать ввод/секреты и будет отправлен провайдеру при Analyze/Fix.
Выбор Analyze либо Fix с последующим submit, включая пустой, разрешают передачу
**именно показанного sanitized log**. Явный headless analyze/fix flag разрешает такую же
передачу без UI, с этим
предупреждением в help. Не обещать отсутствие arbitrary passwords в artifact или bundle.
Это не защита от враждебного same-UID процесса или утечки в исходном live output.

Capture/memory bounded и не блокируется из-за переполнения ring. Известная потеря данных
из-за capture error отличается от намеренного truncation: первая запрещает recovery и даёт
125; второе видно пользователю и допускает recovery. Full capture не дублируется в slog,
SQLite, FTS или embeddings. Собственные diagnostic messages и пути также terminal-escaped.

## 8. Continuation, provenance, tools и рекурсия

### 8.1. Handoff wrapper и обычный continue — разные входы

Обычный `zev continue` сохраняет прежний контракт: пользователь **сам прикрепляет лог**.
Команда не ищет run artifacts, не выбирает failure attempt и не отправляет сохранённый
лог автоматически. В первой версии нет отдельного публичного входа продолжения по run ID,
attempt-selector или флага отправки сохранённого лога. Его самостоятельные exit codes
не меняются этой функцией. Уже прикреплённый пользователем вручную лог обрабатывается
по существующим правилам continue/history, не по запрету автоматического capture-copy.

Автоматическое прикрепление относится только к **внутреннему handoff текущего wrapper**
после выбора Analyze, Fix с submit Clarify request либо явного `--on-failure=analyze`/
`--on-failure=fix`. Он получает выбранный режим и
immutable bounded snapshot **точной текущей неуспешной попытки** и точные IDs, уже известные
runner. Не перечитывает log по pathname, не использует LastConversation и не выбирает latest
attempt. После успешного retry, сигнала, timeout или unknown outcome handoff не вызывается.
Нельзя автоматически возобновить его после завершения процесса, crash или ручного continue.

Bundle v1: run/attempt IDs и ordinal; UTC start/duration; started; outcome/error_source;
exit_code nullable и signal separately; sanitized command representation; bounded log с labels,
truncation/redaction/echo metadata; conversation/command IDs при связи; mode analyze/fix;
optional clarification (для Fix).
Manifest фиксирует command snapshot digest (не authorization), IDs и local cwd snapshot.
Запуск хранит raw argv/environment только в памяти для retry. Artifact содержит sanitized
command, не отдельную raw argv/env копию. При доступной истории сравнивается digest исходной
команды и принадлежность ID; mismatch — fail closed. Проверка не даёт права выполнять команду.
History row не источник attempt outcomes и не перезаписывается конкурентными run.

Самостоятельный run без conversation link использует собственный диагностический контекст
из **memory bundle**, не требует существующей LLM history и не предупреждает об отсутствии
несуществовавшей связи. Если link был задан, но source conversation недоступен, причина
показывается явно и создаётся отдельный контекст без загрузки другого диалога.
Даже в таком новом разговоре history сохраняет только reference/summary без полного лога;
provider получает log только из bounded memory snapshot. History/storage warning не выдумывает
provenance. В active in-process handoff доступен exact диалог в памяти; history-disabled
inject такую память после завершения первого query-процесса не восстанавливает.

### 8.2. Trust boundary и отсутствие копии логов в history

`command_error` Kind сам по себе не wire-граница: текущие builders передают Content.
Нужен единый recovery serializer: JSON-encoded data envelope с фиксированным типом/версией,
строковыми command/error/log, корректным escaping и trusted instruction, что эти поля —
данные, не инструкции. Все пять provider paths, включая tool-enabled builders и Ollama,
MUST сохранять framing; враждебный log не может закрыть envelope или стать system message.
Framing не делает LLM immune к prompt injection; фактическая защита side effects — approval
и host-side tools policy, не обещание «модель никогда не послушает лог».

History сохраняет command_error **reference/summary** (run/attempt IDs, outcome, digest),
не full log, а provider получает log из проверенного memory snapshot. Blank clarification
не становится user refinement; synthetic instruction только в request construction.
При history restore run reference не раскрывается в log автоматически; недоступный artifact
не заменяется чужими данными. Не добавлять log в embeddings/FTS через копию history metadata.

### 8.3. Recovery tools policy

- Default — tools disabled, независимо от TOOLS_ENABLED/TOOL_APPROVAL_MODE config.
- Явные текущие `--tools` разрешают только существующие read-only manual/auto операции;
  default при включении manual. `--no-tools` запрещает; конфликт flags — usage error.
- Явный bypass в run отклоняется до запуска/provider; config bypass игнорируется.
  Правила обычного standalone continue этой функцией не меняются.
- `--no-context` убирает automatic environment context, но не отменяет явно разрешённые
  tools. Help предупреждает, что explicit tools могут раскрывать прочитанные данные.
- Headless manual запрос получает отказ, не ждёт ввода и не становится auto.
- Явный tools intent исходного query переносится в generated run только при безопасном
  manual/auto режиме. Query с bypass продолжает свой прежний контракт, но recovery tools
  для него отключены с предупреждением; bypass не пересекает wrapper boundary.

### 8.4. Рекурсия

Typed execution context ограничивает in-process recovery depth одним handoff. Межпроцессно
runner передаёт служебный depth marker во всё child environment. Cooperative nested run,
включая запуск через shell, подавляет свои retry/menu/provider и возвращает command outcome
внешнему owner. Это correctness protocol, не security boundary. Если child явно удаляет
marker/environment, новый Zev считается независимым invocation; абсолютная защита от
такого намеренного разрыва контекста не обещается. Невалидный/исчерпанный marker подавляет
recovery, не даёт дополнительных прав. Provider/tool/editor/cleanup paths не оборачиваются.
Consented correction run явно создаётся owner как отдельный ID с recovery disabled.

## 9. Durable artifacts, quota и crash recovery

History enabled разрешает `~/.zev/runs/<run_id>/`; history disabled/no-history запрещает
**создание** SQLite/run artifacts. ID — UUID, не путь. Unix directories 0700/files 0600;
Windows ACL — native gate. Проверять nofollow/regular-file identity/link count, размер,
версию и safe components при чтении/публикации; trusted HOME ancestor aliases допустимы.

**Единица публикации attempt:** один immutable versioned JSON envelope с metadata и
sanitized log. Write temp → fsync → rename → directory fsync. Отдельный атомарный manifest
ссылается только на уже committed attempt files и их digests. Сначала attempt, затем manifest;
нет обещания multi-file atomic transaction. Читатель принимает только завершённый manifest
с валидными ссылками. Unreferenced committed attempts после crash не подхватываются как latest.
`ErrCommitted` не означает rollback: видимое новое состояние сохраняется с uncertain durability,
recovery запрещён, outcome wrapper failure/125. Ошибка SQLite summary — отдельное предупреждение,
не ошибка capture/artifact и не повод повторить child.

Manifest lifecycle: active → finished; исчезновение владельца до finished → crashed/unknown.
Owner удерживает OS-backed exclusive per-run lock, reader — shared lease до bounded snapshot,
GC пытается получить exclusive lock без ожидания. Создание run и приобретение reader
lease координируются тем же budget lock, чтобы GC не удалил путь между lookup и lock.
PID/time/heartbeat не доказывают liveness.
Kernel release lock после crash позволяет reclaim без PID reuse эвристик. Recovery crashed
run автоматически не возобновляется; incomplete staging и orphan attempts reclaim-ятся под lock.
Lock inodes не unlink/recreate-ятся, пока они могут быть идентичностью живого lease.

**Quota:** 100 MiB — общий предел логических payload bytes и reservations внутри runs,
не обещание о filesystem block allocation/metadata. Учитываются committed/staging attempt
files и manifests; external events-file исключён. Attempt metadata ≤64 KiB, manifest ≤64 KiB.
До первого launch резерв под межпроцессным budget lock:
`2 × ((max_retries + 1) × (log_limit + 65536) + 65536)` байт.
Множитель 2 покрывает сосуществование staging/committed generation. Все allocation/update/GC
соблюдают reservation; **budget lock** не удерживается во время child/provider I/O. Owner
сохраняет **per-run lock непрерывно** через child/provider до finalization и завершения всех
своих artifact writes. Короткие
budget операции не ждут per-run lock: GC только try-lock. Reservations stale owners
reconcile-ятся под budget lock с kernel lock evidence. Малый размер reservation также
ограничивает число одновременно сохраняемых runs; отдельного unbounded registry нет.
Активный run учитывается своей reservation, а не суммой reservation плюс уже записанные
ею bytes. После завершённой finalization под budget lock charge заменяется фактическим
размером retained файлов, освобождая неиспользованный резерв; GC освобождает остаток.

Если резерв не помещается, сначала GC завершённых unlocked runs по oldest-finished, затем
выбрать **весь invocation ephemeral до первого child launch**, с предупреждением. Частично
переключать persistent run после write failure нельзя. Например 16 MiB × 11 attempts не
помещается и заранее становится ephemeral, но in-memory recovery допустим. Фактический
ENOSPC/write/fsync failure даже при reservation — wrapper error/125, без recovery.
Retention 7 суток применяется при startup/access/GC, не фоновым daemon; активные/read-leased
runs не удаляются до освобождения lock. После срока и освобождения lease reclaim допустим;
retention не является механизмом автоматической передачи сохранённого лога в LLM.
Deletion не гарантирует forensic erase,
удаление backups или текста, уже отправленного провайдеру.

## 10. Exit codes и отдельные результаты

Сначала вычисляется **command result**: original = первая попытка, last = последняя
фактически выполненная попытка исходной команды. None, analyze/fix response/failure, отказ от
исправления и результат consented correction не меняют эту выбранную policy.
Start error считается attempt с результатом 126/127; cleanup/capture — не обычный command exit.

Приоритет возвращаемого результата (сверху вниз):

| Условие | Код |
|---|---|
| Supervision/capture/artifact/display/events failure или cleanup_unknown | 125 |
| Полученный wrapper SIGINT / SIGTERM, в том числе в menu/delay/provider | 130 / 143 на Unix |
| Истёкший per-attempt wrapper timeout | 124 |
| Child-only signal без parent cancellation | 128 + signal на Unix |
| Аргументная ошибка до launch | 2 |
| Иначе | command result по original/last; при первом успехе 0 |

Первая зарегистрированная parent cancellation фиксирует SIGINT/SIGTERM; поздний сигнал
не меняет выбранную cancellation reason. Cleanup failure всё равно повышает её до 125.
Timeout retry важнее original failure. Parent cancel во время recovery возвращает cancellation
code, а обычная provider error сохраняет command-result policy. Все вторичные причины
сохраняются в metadata; не выбирать причину по порядку errors.Join. После финализации
run_finished результат неизменяем, более поздние события не принадлежат завершённому run.
Windows mapping публикуется и тестируется отдельно; Unix-формула не копируется механически.
Одинаковые числовые child/wrapper коды различаются outcome/error_source, не угадываются.

## 11. События, архитектура и platform gates

`--events-file` — явное исключение из no-history: разрешает только metadata JSONL даже
при `--no-history`, без SQLite/run artifacts/log copies. Help описывает это сочетание.
Exclusive create 0600, no overwrite/symlink/nonregular; один writer, ordered sequence.
Для external events path проверяются **все** компоненты: directory-relative traversal
от pinned cwd/выбранного абсолютного anchor, уже существующие реальные parent directories,
exclusive nofollow leaf и проверка opened identity. Symlink в выбранном пути и подмена
parent между проверкой/open отклоняются, запись через него не продолжается. Доверенные
OS/HOME aliases допускается канонизировать заранее в composition root, но не применять
такой fallback к непроверенным path components. Родители автоматически не создаются.
Каждая encoded line ≤8 KiB, весь events-file ≤1 MiB; исчерпание/short-write error даёт 125
и запрещает recovery. Блокирующий kernel filesystem I/O имеет ограничение §7.2.

Fields: schema_version=1, run_id, attempt_id при наличии, sequence, timestamp, type.
Types: run_started, attempt_started, attempt_finished, failure_log_shown, display_failed,
action_selected, recovery_started, recovery_finished, run_finished.
Action содержит none/retry/analyze/fix и reason (user_submit/cancel/eof/ui_error/headless_default/
explicit_policy/budget_exhausted). Terminal result содержит command-result policy, initial/
last command outcome, correction/recovery outcome отдельно, returned code и secondary causes.
Без argv/cwd/содержимого логов/секретов. События не являются authoritative recovery journal.

При нормальном завершении и исправном sink run_finished **эмитится один раз**. Crash может
оставить отсутствие terminal event/частичную последнюю строку: читатель игнорирует только
оборванный tail, не приписывает ему успех. Fsync-durability/event replay/exactly-once delivery
через crash не обещаются. При неисправном sink сообщение best-effort в stderr.
Slog содержит только stable names/IDs. EN/RU UI локализуется, machine keys/IDs/outcomes — нет.

Срез run/service владеет политиками и consumer ports, repository/client — adapters;
CLI создаёт зависимости, cmd/zev остаётся launcher. Childprocess остаётся общим owner,
terminal-session adapter расширяет необходимый контракт ownership, не создаёт второй supervisor.
AI-обработка выделяется в общий typed use case без копирования независимого agent loop.
Analyze имеет отдельный consumer-side provider contract/validator для FailureAnalysis и
свою instruction/schema во всех пяти adapters; он переиспользует transport и serializer,
но не маскируется под генерацию Command. Provider factory вызывается лениво только для
явной AI-ветки. Existing query OptionsResponse API и обычный continue не меняются.
SQLite schema не меняется, existing history execution summary не используется как attempt store.
Новые terminal dependencies требуют отдельного согласия и проверки official docs/версий.

Initial native acceptance: Linux и macOS. На Windows default wrapper не включается до
native TTY/process/ACL gate; запрос wrapped delivery сообщает unsupported до исполнения,
явный no-wrap сохраняет прежний путь. Cross-build не является native evidence. Точный
support matrix и platform mapping фиксируются реализацией до объявления поддержки.

## 12. Критерии приёмки и проверки

| ID | Проверяемое поведение |
|---|---|
| A01 | Exit 0 со stderr не вызывает recovery |
| A02 | Failure log опубликован до меню/retry/provider; default меню none |
| A03 | Enter в initial action menu с выбранным «Ничего не делать», Escape/EOF/ошибка этого меню не вызывают recovery; это не правило Clarify submit |
| A04 | Headless default не читает ввод и ничего не делает |
| A05 | Explicit headless retry/analyze/fix соблюдают policy и display barrier |
| A06 | Manual failed retry возвращает ask-menu с none; auto budget не эскалирует в analyze/fix |
| A07 | Retry сохраняет launch snapshot и ждёт cleanup; input pipe не воспроизводится |
| A08 | Bounded binary/large output, chunk redaction, escaped controls и visible truncation |
| A09 | Empty output/start error видны; error-before-start не путается с запущенным child |
| A10 | Cancel/timeout/child signal не запускают recovery; cleanup failure даёт 125 |
| A11 | Реальный subprocess проверяет original/last для [7,9], none/analyze/fix/provider failure |
| A12 | Exact run/attempt/conversation при двух терминалах; никакого latest fallback |
| A13 | No-history не открывает SQLite/не создаёт artifacts; in-memory Analyze/Fix работают |
| A14 | Repository отвергает invalid/missing/incomplete/symlink artifacts; их наличие не запускает автоматическую отправку в LLM |
| A15 | Config execute/bypass не даёт recovery execution/tool authority |
| A16 | Реальный nested run и shell-nested run подавляют recovery; env removal имеет документированную границу |
| A17 | Wrapped inject/execute/fallback сохраняют shell text одним argv и executable path |
| A18 | Machine query raw; entity execution без auto-wrapper; opaque alias tail не парсится как query |
| A19 | Data stdout чист; metadata events ordered/bounded, terminal emission без crash-гарантии |
| A20 | Concurrent quota reservation/GC не превышают budget и не удаляют active/read-leased runs |
| A21 | EN/RU help/screens/defaults и parent-shell limitation согласованы |
| A22 | Native PTY input/resize/Ctrl-C/termios/descendant cleanup на каждой поддерживаемой ОС |
| A23 | Все root builtin names/aliases/hidden/help/completion запрещены во всех alias create/rename путях |
| A24 | Fix открывает Clarify request и автоматически прикладывает log/error |
| A25 | Empty/whitespace submit вызывает provider без пустого исторического refinement |
| A26 | Clarify cancel/EOF/UI error Fix не отправляют запрос; headless fix пропускает экран; Analyze не требует Clarify |
| A27 | Registry coverage и существующие name conflicts не допускают тихого shadowing/rename |
| A28 | Inject переносит effective privacy flags при смене config и не восстанавливает утраченную память |
| A29 | Обычный continue требует ручного прикрепления лога; внутренний wrapper handoff не использует LastConversation или paste-log questionnaire |
| A30 | Correction preview не исполняет/не вставляет; default confirmation отказ; consented run без recovery |
| A31 | Broken stderr/display запрещает recovery; UI-init и mid-Clarify failure различаются |
| A32 | Child payload bytes неизменны при redirects; stdout чист, stderr дополнен отделённой plaintext диагностикой; TTY stdin не включает PTY/TUI для redirected stderr |
| A33 | Receiving shell не выводится из SHELL; quotes/$()/backticks/Unicode/LF сохраняют границы |
| A34 | Current Zev path работает без PATH entry, при spaces/quotes/другом zev в PATH |
| A35 | PTY echo_possible и disclosure warning; fixture не обещает arbitrary-secret elimination |
| A36 | Все provider/tool builders сохраняют escaped untrusted envelope; default tools не выполняют log instructions |
| A37 | Recovery tools matrix config bypass/no-context/explicit tools/manual/auto/headless fail-closed |
| A38 | Crash до/после attempt/manifest rename, ErrCommitted, stale locks и reservations корректно классифицируются |
| A39 | Внутренний handoff использует snapshot текущей failed attempt, не предыдущую/latest из history; success/unknown не исправляются автоматически |
| A40 | No-history + explicit events-file пишет только ограниченные metadata; short write/partial crash tail учтены |
| A41 | При смешанных cancellation/deadline/capture/cleanup причина и returned code следуют одной таблице |
| A42 | Wrapper log не копируется в history/FTS/embeddings и в новый diagnostic dialogue при missing conversation; manual continue attachments сохраняют прежний контракт |
| A43 | Budget lock свободен во время blocked child/provider, per-run lock защищает run/reservation от GC до finalization |
| A44 | Recovery deadline отменяет provider/tool loop без исполнения исправления; сохраняется command policy, parent cancellation/cleanup failure приоритетнее |
| A45 | Events path с symlinked/replaced parent не создаёт файл через небезопасный путь; trusted anchor aliases не ослабляют проверку suffix |
| A46 | Явный run выполняет пользовательскую команду без query, conversation IDs и существующей LLM history |
| A47 | Success/none/retry не создают provider и не проверяют missing/invalid LLM settings; provider call возможен только после AI consent/display barrier |
| A48 | FailureAnalysis строго валидируется по отдельной schema, bounded/escaped; причины связаны с evidence и неизвестность не маскируется |
| A49 | Analyze с command-like/hostile ответом не вызывает Execute/Inject/selector, не меняет файлы и не входит в Fix |
| A50 | Четыре пункта меню с default none; analyze/fix flags различимы, старое agent отклоняется; Clarify только для Fix |
| A51 | Request/event/manifest mode не меняется ответом модели; все пять provider paths поддерживают analysis contract без фиктивных Command |
| A52 | Анализ успешен/неуспешен/deadline не меняет original/last command result; report не копируется в history вместо log/commands |

Unit tests рядом с кодом: testing/table-driven, существующий стиль assertions; RED до
реализации. Provider fixtures локальные, без реальных ключей/LLM. Process tests используют
реальный child/built binary, а не сигнал тестовому runner. Native integration подтверждает
PTY и kernel ownership отдельно от compile/host tests. Нельзя заявлять отсутствие arbitrary
kernel hangs или очистку отделившихся процессов по зелёному mock-тесту.

```bash
go test ./internal/cli ./internal/core/childprocess ./pkg/terminal
make test
make test-race
make lint
make test-integration
make test-e2e
make build
make clean
```

Run-slice focused tests обязательны после появления пакета. Manual — только с отдельного
разрешения. Перед поставкой синхронизируются README, CONFIGURATION, ARCHITECTURE, PROJECT,
help/setup, locale catalogs и CHANGELOG. Для prose-only ревью — ссылки, whitespace,
CLI/default/acceptance parity и fence checks; не запускать autofix как проверку документа.

## 13. Статус согласования и ревью

Пользователь согласовал feature scope, Clarify flow, reserved names и defaults.
Проведены два раунда независимого read-only ревью: четыре reviewer в первом и три во
втором, с разделением UX/CLI/storage/security/process. Подтверждённые замечания устранены
в обеих версиях; после второго раунда уточнены stderr, lifetime locks, recovery deadline
и events paths. По прямому уточнению пользователя убран лишний публичный run-continuation
путь: обычный continue остаётся ручным. Это устраняет две связанные с ним продуктовые
неоднозначности, а не назначает им невыбранные пользователем defaults.

Финальные правки проверены автором по целевым контрактам и документационными проверками;
отдельный третий независимый раунд после них не запускался. Ревью спецификации не является
верификацией будущего runtime и не доказывает отсутствие всех возможных дефектов.

Это specification-only работа без кода, коммита, миграций или установки зависимостей.
Новые продуктовые развилки не считаются согласованными только по совету reviewer.

После этих раундов пользователь добавил Analyze без исправления и закрепил самостоятельный
run наравне с query. Четыре действия, новые analyze/fix policies, FailureAnalysis и lazy
provider setup синхронизированы в EN/RU и проверены автором. Это расширение не покрывается
утверждением о завершённых независимых ревью предыдущей версии. Самостоятельный run остаётся частью одной функции выполнения и обработки ошибок,
а не обещанием чтения shell history или полностью автономного исправления.

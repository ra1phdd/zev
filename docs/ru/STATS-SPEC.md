<div align="center">

[English](../en/STATS-SPEC.md) | **Русский**

</div>

# Спецификация: zev stats — токены, активность и контекст

**Статус:** черновик для согласования. Функция не реализована; документ не разрешает реализацию, миграцию пользовательских данных, новые зависимости или коммит.

## 1. Цель и основание

Пользователь хочет команду `zev stats`, похожую на статистику Claude Code: красивый терминальный интерфейс с графиками и понятным расходом контекста.

Успех: пользователь видит расход токенов за период, распределение по моделям и размер входа последнего запроса выбранного диалога, не путая эти показатели. Данные локальные, просмотр не требует API-ключа или сети.

### Подтверждённое текущее состояние

- [InferenceProvider](../../internal/core/domain/provider.go) возвращает `OptionsResponse` без usage; [ToolTurn](../../internal/core/domain/tool.go) также не содержит usage.
- Адаптеры [OpenAI](../../internal/entities/query/repository/openai/provider.go), [Anthropic](../../internal/entities/query/repository/anthropic/provider.go) и [Gemini](../../internal/entities/query/repository/gemini/provider.go) не передают usage потребителю. [Ollama](../../internal/entities/query/repository/ollama/provider.go) использует OpenAI-адаптер.
- [OpenCode](../../internal/entities/query/repository/opencode/provider.go) запрашивает `include_usage`, но текущий SSE-парсер извлекает только текст.
- [Tool service](../../internal/entities/tool/service/service.go) выполняет несколько model turns и отдельный safety review. Финальный ответ не представляет весь расход.
- [History lifecycle](../../internal/entities/history/service/lifecycle.go) сохраняет диалоги и ответы, но не журнал usage. Отключённая история не открывает SQLite.
- Точка входа — [cmd/zev/main.go](../../cmd/zev/main.go), routing/composition — [internal/cli](../../internal/cli/root.go). Архитектура — [vertical slices + ports/adapters](ARCHITECTURE.md).

Это наблюдения по checkout, а не обещание совместимости с параллельными незавершёнными изменениями. Перед реализацией точки интеграции проверяются заново.

### Предлагаемые defaults, требующие утверждения

1. Один функциональный контракт `stats`: сбор usage, локальная агрегация и представление — части одной функции, не отдельные продукты.
2. Статический дашборд по умолчанию; интерактивность только через `--interactive`.
3. Период по умолчанию — последние 30 календарных дней, включая сегодня; часовой пояс процесса.
4. Статистика следует существующей политике истории; отдельного скрытого хранения нет.
5. Первая версия без стоимости, лимитов подписки и приблизительного токенизатора.
6. Контекст означает **вход последнего наблюдённого основного запроса**, не гарантированный размер следующего запроса.

MUST/MUST NOT ниже — проверяемые требования предлагаемого контракта, а не описание реализованного поведения.

## 2. Область первой версии

Входят: пять существующих query-провайдеров; обычный query, machine query и `continue`; промежуточные tool turns и safety review; локальный журнал наблюдений; календарь; дневной/недельный график; модели; cache/reasoning breakdown; выбранный диалог; JSON; русский/английский UI.

Не входят: стоимость и прогнозы, квоты аккаунта, импорт статистики других CLI, облачная синхронизация, web dashboard, project filters, достижения/streak, полный интерактивный браузер истории, автоматическое сокращение контекста, изменение промптов и safety policy. Embeddings, metadata generation и ещё не реализованные возможности из других спецификаций не входят в totals первой версии. В UI/JSON область называется `query_llm`, а не «все расходы Zev».

Прямое выполнение alias/script без LLM не увеличивает calls/tokens. Named machine lookup без LLM тоже не увеличивает их. Независимое внедрение run/update/metadata-функций не является предусловием stats.

## 3. Термины и числовой контракт

| Поле | Значение |
|---|---|
| `input_tokens` | Полный вход провайдера, включая cache read/write, если они входят во вход этого протокола |
| `output_tokens` | Полный сгенерированный выход, включая reasoning, если он включён протоколом или может быть достоверно добавлен |
| `cache_read_tokens` | Подмножество `input_tokens`, прочитанное из кэша |
| `cache_write_tokens` | Подмножество `input_tokens`, записанное в кэш |
| `reasoning_tokens` | Подмножество `output_tokens`, использованное для reasoning |
| `total_tokens` | `input_tokens + output_tokens`, только когда оба известны |
| `provider_total_tokens` | Отдельное исходное числовое total провайдера для проверки согласованности, не слагаемое |
| `calls` | Число зарегистрированных логических обращений к query-адаптеру, не число prompts, tools или HTTP retry |
| `conversations` | Число различных диалогов с зарегистрированными calls в диапазоне |

Все счётчики — неотрицательные целые `int64`, неизвестное значение — `null`, известный ноль — `0`. Отсутствие, JSON null, отрицательное, дробное, переполнение и некорректный тип MUST NOT превращаться в известный ноль. Невалидная метрика получает `null` и безопасный код диагностики; валидные независимые метрики сохраняются.

`cache_read_tokens`, `cache_write_tokens` и `reasoning_tokens` MUST NOT добавляться к уже включающим их входу/выходу. Нельзя придумывать недостающие поля вычитанием несогласованных totals. При известном нарушении subset/total-инварианта затронутый набор метрик исключается из точного подсчёта и отмечается invalid; варианты команд от этого не становятся невалидными.

### Нормализация протоколов

| Протокол | Нормализованный вход | Нормализованный выход |
|---|---|---|
| OpenAI Chat Completions | `prompt_tokens`; cached tokens — подмножество | `completion_tokens`; reasoning — подмножество |
| Anthropic Messages | `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` при известных требуемых компонентах | `output_tokens`, без повторного добавления thinking |
| Gemini generateContent | `promptTokenCount`, уже включающий cached content | `candidatesTokenCount + thoughtsTokenCount`, если семантика и наличие компонентов подтверждены |
| OpenCode SSE | Нормализация OpenAI-compatible usage из финального usage-события | То же; событие без `choices` тоже обрабатывается |
| Ollama compatible endpoint | OpenAI-compatible usage реально полученного ответа | То же, без предположений о полноте endpoint |

Отсутствующий optional field можно считать нулём только если официальный контракт конкретного протокола это гарантирует; compatible URL сам по себе такой гарантии не даёт. На этапе реализации семантика сверяется с закреплёнными SDK и официальной документацией. Дополнительные Gemini-категории, не покрытые выбранной формулой, не скрываются: расхождение с provider total отмечается как несогласованность, а не принудительно распределяется.

### Расход и контекст

- Расход за период/диалог — сумма **известного нормализованного usage отдельных calls**. Повторно отправленная история учитывается при каждом обращении.
- `last_request_input_tokens` — вход последнего основного call диалога. Если он неизвестен, показывается неизвестное, а не старое известное значение.
- `context_window_tokens` — достоверный лимит окна той же модели/endpoint на момент call. Источник лимита сохраняется. Не выводится из имени модели или максимума output. В первой версии лимит может быть неизвестен для всех существующих адаптеров.
- `input_window_percent = 100 × last_request_input_tokens / context_window_tokens`; допустим только при обоих известных значениях и положительном лимите. Название: «Вход последнего запроса / окно». Это не свободный бюджет генерации; выход и резерв требуют отдельной политики.
- Превышение 100% не скрывается: бар ограничен по ширине, рядом настоящее число и предупреждение. Пустой контекст не изображается как 100% свободного.
- Safety review увеличивает расход, но никогда не заменяет snapshot основного контекста. Tool turns основного диалога могут заменять его.
- После завершения процесса snapshot исторический. Сохранённая история содержит сокращённые ответы и не позволяет точно восстановить следующий wire request. В UI показываются время snapshot и модель.

## 4. CLI

```bash
zev stats
zev stats --days 7
zev stats --year 2026
zev stats --all
zev stats --provider anthropic --model claude-example
zev stats --session last
zev stats --session <conversation-id>
zev stats --days 30 --json
zev stats --interactive
```

`claude-example` — условный идентификатор, не обещание доступности модели.

| Флаг | Default | Контракт |
|---|---|---|
| `--days N` | `30` | Целое 1–366; N календарных дат до сегодня включительно |
| `--year YYYY` | не задан | Четырёхзначный год 1970–9999; не позже текущего |
| `--all` | `false` | Вся сохранённая область query usage |
| `--provider ID` | не задан | Точное совпадение configured provider ID: openai/gemini/anthropic/ollama/opencode |
| `--model ID` | не задан | Точное совпадение сохранённого request model ID, регистр сохраняется |
| `--session ID\|last` | не задан | Один диалог; несовместим с периодом, provider/model filters |
| `--json` | `false` | Один JSON-документ schema version 1, без ANSI/TUI |
| `--interactive` | `false` | Полноэкранный TUI, только при TTY stdin/stdout |

Явные `--days`, `--year`, `--all` взаимоисключающие. `--json` и `--interactive` несовместимы. Позиционные аргументы, неизвестные флаги, пустые ID/filters, повтор одного value-флага и числовые значения вне диапазона — usage error до открытия БД. Несуществующая комбинация provider/model даёт пустой отчёт, не provider lookup. `--interactive=false` не включает TUI.

`stats` — built-in с приоритетом над named entities. Новое имя запрещается для новых/переименованных aliases/scripts через общую write-time валидацию, с тестом согласованности с корневыми командами; текущая [валидация metadata](../../internal/core/metadata/validation.go) использует статический список, готовый динамический registry не предполагается. Чтение существующего alias/script `stats` MUST NOT делать всё хранилище нечитаемым: запись сохраняется, доступна для list/edit/remove, но корневой `zev stats` теперь принадлежит built-in. При обнаружении коллизии выводится безопасное предупреждение с инструкцией изменить имя; автоматического удаления/переименования нет. Это предлагаемое изменение приоритета, подлежащее утверждению с draft. Хвост аргументов иных aliases/scripts сохраняет прежний opaque argv-контракт.

### Диапазоны и выбор last

- Текущее время фиксируется один раз при открытии отчёта; время в БД — UTC, границы дат рассчитываются в системной timezone с учётом DST, не через N × 24 часа.
- Интервал `[from, to)`; `to` — зафиксированное now. Для прошлого года `to` — 1 января следующего года. Future records не учитываются.
- Фильтрация calls по `started_at`; завершившийся после полуночи call относится к дате начала. Незавершённый зарегистрированный call учитывается как call, но не как известный token total.
- `last` выбирает последний сохранённый query-диалог по `(created_at DESC, id DESC)`, даже без usage. Старый диалог показывает «usage не записывался», не перескакивает к более старому с числами. Продолжение считается тем диалогом, который реально создаёт существующий `continue`; унаследованные сообщения не являются новыми calls.
- Явная команда stats может читать существующую историю при `HISTORY_ENABLED=false`: настройка запрещает сбор в query, но не явный локальный просмотр. Настройка не включает запись задним числом.

### Потоки и ошибки

Stdout — отчёт либо JSON; stderr — безопасная диагностика. Progress, provider configuration и сетевые вызовы отсутствуют. JSON сначала формируется целиком; до ошибок аргументов/чтения stdout пуст. Ошибка записи stdout может оставить неполный документ и возвращает неуспех.

Коды: `0` — отчёт, включая пустой; `1` — storage/I/O/unsupported schema/session not found; `2` — аргументы или неподходящий TTY для явной интерактивности; `130` — отмена Ctrl-C. `q`/Escape в TUI — штатный выход `0`. `--help` не открывает историю и не запускает миграции. Изменения кодов других команд не входят в задачу.

## 5. Представление

### Статический обзор

Порядок блоков: заголовок периода/timezone/scope → summary cards → heatmap → token bars → models → coverage/limitations. Cards: conversations, calls, известный total/input/output. Cache/reasoning показываются как «в том числе», не отдельные слагаемые total.

- Heatmap по умолчанию измеряет известные tokens/day; легенда явно называет единицу. В интерактивности можно переключить на calls/day.
- Уровни heatmap: 0; затем четыре равных интервала относительно максимума положительных значений видимого диапазона (`ceil(4 × value / max)`). Изолированный пик не обрезается. Пороги доступны в legend/JSON; переключение периода меняет шкалу.
- Дата без calls — отдельное пустое состояние. Calls с полностью неизвестными tokens — `?`, а не нулевой расход. Частичные данные — известная сумма плюс видимый marker. Future dates — пробелы, не нулевая активность.
- Для `--days` bars дневные; для `--year`/`--all` — недельные с понедельника. Неполные недели не дополняются данными вне диапазона. Heatmap и weekly bars показывают максимум последние 52 недели; карточки остаются за весь запрошенный период. Подпись раскрывает отличие диапазона графика от totals.
- Models: группировка `(configured provider, request model)`, сортировка по known token total DESC, затем provider/model ASC. Первые 5 и строка остальных, без потери сумм. Группы без известных tokens следуют после известных. Имена экранируются от terminal control sequences.
- Доля кэша = известные cache-read tokens / известные input tokens **только на calls с обоими полями**. При нулевом знаменателе `n/a`; показывается размер покрытой выборки. Это доля токенов, не процент запросов с cache hit.

Ширины для snapshot tests: 120, 80, 40 и 20 колонок. При >=100 cards могут стоять в строку; при 60–99 — компактный layout; при <60 — вертикальные метрики и последние помещающиеся недели с подписью. При <30 календарь заменяется текстовым summary. Никаких выходов за доступную ширину, разрезания UTF-8 или зависимости смысла только от цвета.

Один акцентный цвет, приглушённые подписи, выровненные числа, минимум рамок. `NO_COLOR`, `TERM=dumb` и не-TTY отключают ANSI. Не-TTY не открывает TUI и выводит текстовые таблицы вместо графической сетки. Русская/английская локаль следует существующему `ZEV_LANG`; flags, JSON keys и пользовательские model IDs не переводятся.

### Диалог

Сначала ID, модель/модели, дата и coverage; затем вход последнего основного call/лимит, при известном лимите — горизонтальный бар; затем накопительный расход и график input по основным calls в порядке старта. Safety review показан отдельным расходом. Пропуски на графике не соединяются как нули. Диалог без calls имеет пустое состояние.

### Интерактивность

`Tab` — обзор/модели; `m` — tokens/calls; `Left`/`Right` — дата; `Enter` — локальные числовые детали дня; `Escape` — закрыть детали, затем выход; `q` — выход. `7`/`3`/`y`/`a` выбирают 7 дней/30 дней/текущий год/всё время; в session view эти переключатели не действуют. Terminal resize сохраняет выбранную дату, если она помещается в новый диапазон.

Первый экран основан на согласованном storage snapshot. Автоматического polling нет; `r` открывает новый snapshot и обновляет now. Переключение периода может перечитать БД, но не вызывает провайдера. Навигация никогда не запускает команды, не раскрывает промпты и не изменяет историю. TUI восстанавливает терминал после выхода/ошибки/отмены.

## 6. Сбор, хранение и совместимость

### Журнал calls

Предлагается additive migration в существующей SQLite history, без отдельной БД/JSONL. Точный следующий migration number выбирается по актуальному checkout. SQL-DDL и Go signatures определяются после утверждения спецификации; ниже обязательный логический контракт.

Call содержит: локальные `call_id`, `conversation_id`, `sequence`, `purpose=primary|safety_review`, configured provider, request model, UTC started/completed timestamps, `status=started|succeeded|failed|cancelled`, nullable нормализованные counters, provider total, безопасные diagnostic codes и nullable context limit с provenance. `sequence` задаёт полный порядок calls одного диалога, в том числе review. Endpoint URL, API keys, headers, prompts, tool arguments/results и raw responses в journal не сохраняются.

- Configured provider сохраняется отдельно от adapter `Name()`: Ollama не превращается в openai-compatible. Request model — фактически отправленный ID, не текущий config при просмотре.
- Начало call регистрируется непосредственно перед вызовом adapter, если история доступна. Отказ в tool execution сам по себе не является LLM call; запрос модели, получивший отказ в виде tool result, является.
- Usage фиксируется до JSON options decoding и последующей бизнес-валидации: полученный платный, но невалидный ответ тоже входит в наблюдённый расход. Исходный статус результата команды не подменяется status сетевого call.
- Финализация обновляет ту же запись, а не добавляет вторую. Повтор одинаковой финализации идемпотентен; конфликтующая финализация не перезаписывает прежнее наблюдение и даёт безопасную диагностику.
- Usage сохраняется по мере получения, а не только после успешного завершения всего query. Fail/cancel не удаляет уже известный usage; crash оставляет started/unknown.
- Повторные stream events MUST NOT многократно суммировать cumulative snapshot. Способ замены snapshots/суммирования deltas определяется контрактом протокола; final usage без content обязателен к обработке. Оборванный stream помечается partial/unknown согласно реально наблюдённым полям.
- HTTP transport retries — попытки внутри одного логического call, не дополнительные prompts. Невидимое клиенту потребление при retry/timeout не восстанавливается. Если retry был, сохраняется признак `transport_retried`, и полнота provider billing не обещается. Захват usage не должен повторять запрос ради статистики.
- Сбор охватывает основной adapter path и ToolProvider path. Обёртка вокруг финального OptionsResponse недостаточна. Один primary call не записывается дважды через разные слои.

Ошибки journal не блокируют получение/доставку команды: безопасное предупреждение в stderr, без network retry и без загрязнения machine stdout. Если нет созданного durable conversation, collector не создаёт orphan usage или альтернативное хранилище. До открытого network request отсутствие записи может быть восстановлено лишь новым независимым вызовом пользователя, не автоматическим повтором.

### Coverage и границы точности

Для записанных calls считаются `known_total_calls`, `partial_calls`, `unknown_calls`; категории непересекающиеся. Known total требует известных input/output; optional details могут оставаться неизвестными. Partial имеет хотя бы один известный token counter, но не total; unknown не имеет валидных counters.

Для каждой метрики агрегат содержит `known_sum` и `known_calls`; доля = known_calls / calls, при calls=0 — null. Known sum по пустой выборке — 0, но при ненулевом calls и known_calls=0 UI показывает `n/a`, не «0 использовано». Input/output sums могут иметь разное покрытие и MUST NOT складываться в total: total суммируется только по calls с обоими полями.

Старые диалоги без call journal отмечаются отдельно как `legacy_conversations_without_call_records`; нельзя оценить число их скрытых tool/review вызовов. Для обзора это query-диалоги, созданные в диапазоне и совпавшие с provider/model filters; метрика не доказывает отсутствие незаписанных вызовов в новых диалогах. `recording_started_at` — время включения версии collector, не гарантия непрерывной записи. Удалённые данные, disabled history, ошибки записи, crash и потери ответов ограничивают статистику. Даже 100% coverage записанных calls не означает полный счёт аккаунта.

### Миграции и чтение

- Новая схема не меняет OptionsResponse JSON, machine query envelope, prompts, сохранённые команды и старые записи. Старые usage не backfill-ятся из длины текста.
- `HISTORY_ENABLED=false`/`--no-history` не открывает БД, не создаёт call records и не сохраняет счётчики в иной файл. `--no-context` не отключает измерение при включённой истории.
- Удаление диалога удаляет связанные call records и изменяет totals; скрытого «вечного» агрегата нет. Унаследованные сообщения `continue` не копируют usage исходного диалога.
- Запись использует существующие транзакции/проверки путей/permissions. Foreign keys, uniqueness и индексы обеспечивают связь/порядок; private mode 0600/0700 на Unix и существующие Windows guarantees сохраняются.
- Чтение выполняется в одном согласованном SQLite snapshot; не держать read transaction всё время жизни TUI. Memory ограничивается агрегатами дат/моделей, не всеми prompts/ответами.
- Отсутствующая БД без legacy — пустой успешный отчёт без создания БД/каталога. Если есть legacy или старая SQLite, stats использует установленный history migration protocol; это может локально изменить storage, но не вызывает сеть. Команда не объявляется строго filesystem-read-only. Миграционные ошибки не скрываются пустым отчётом.
- Неизвестная будущая schema, corrupt DB, denied permissions и busy timeout — видимые ошибки. Нельзя сбрасывать/восстанавливать БД пустой. SQL находится в repository, не CLI/TUI.

## 7. Машинный контракт

`--json` — один объект UTF-8 с числовым `schema_version: 1`, `status: "ok"`, `scope: "query_llm"`. Keys snake_case; идентификаторы/status не локализуются. В первой версии consumers MUST использовать целочисленный decoder: counters могут превосходить точность JavaScript Number.

| Поле | Обязательное содержимое |
|---|---|
| `generated_at` | RFC3339 UTC snapshot time |
| `range` | `mode=days|year|all|session`, nullable `from`, exclusive `to`, timezone name и применённые filters; session ID для session |
| `summary` | conversations, calls, known_total_calls, partial_calls, unknown_calls, legacy_conversations_without_call_records |
| `tokens` | input/output/cache_read/cache_write/reasoning/total; каждый `{known_sum, known_calls}` |
| `daily` | Массив по date ASC: date, calls, token aggregates, unknown_calls, partial_calls; включает пустые прошедшие даты ограниченного range |
| `models` | Все группы provider/model с calls и token aggregates; сортировка как в UI, без top-5 truncation |
| `context` | null в overview; в session — последний primary call ID/time/model/status, nullable input/limit/percent/provenance; null если primary calls нет |
| `coverage` | recording_started_at, список стабильных limitation codes, предупреждения о retry/invalid usage |
| `chart` | Видимый range, metric, bucket unit, heatmap thresholds; график может иметь диапазон меньше totals |

Для `all` daily начинается с первой сохранённой call date, если calls нет — пустой массив. Для session daily покрывает даты его calls. Неограниченный all может давать большой JSON; это явный запрос полного дневного ряда, не обещание фиксированного размера. Overview ограниченных range включает пустые даты до today, но не будущие даты года. Session не экспортирует prompt, command text, error payload или raw provider IDs.

Поддерживаются дополнительные optional keys без изменения версии; изменение семантики/типов требует новой версии. Ошибка не маскируется `status: ok`; до ошибок чтения JSON не печатается, stderr и exit code следуют разделу 4. JSON не содержит UI strings или preformatted числа.

## 8. Архитектура, стиль и зависимости

Стек: Go 1.27.2; существующие Cobra 1.10.2, Bubble Tea 1.3.10, Lipgloss 1.1.0, modernc SQLite 1.62.0. Новые зависимости не требуются; Context7 недоступен во время исходного исследования, поэтому API библиотек при реализации сверяются с официальной документацией и закреплёнными версиями.

- `internal/core/domain`: типы provider usage без UI/persistence; не вставлять usage в model-generated JSON.
- `internal/entities/query/repository/*`: получение/валидация/нормализация usage обоих provider paths.
- `internal/entities/history/repository`: текущий доступ к данным SQLite и journal storage; оркестрация миграции и встроенная база находятся вне entity package.
- Предлагаемый `internal/entities/stats/service`: метрики, coverage, диапазоны, consumer-side storage port; repository adapter к существующей history DB без второй системы открытия/permissions.
- Предлагаемый `internal/entities/stats/transport`: text/JSON/Bubble Tea presentation. Точное разбиение файлов — предмет следующего плана, не утверждённые существующие symbols.
- `internal/cli`: routing, flags, registration, DI и lifetime. Locale keys/catalogs — существующий механизм проекта. Unit-тесты рядом с кодом; black-box тесты — `e2e/` с тегом `e2e`.

Стиль: context первым, named parameter struct для длинных сигнатур, ошибки через `%w`, существующие errors.Is-классификации истории, комментарии только на английском, GoDoc для exports. Nullable counters явные, без magic negative sentinels. Это пример формы, не обещание существующего API:

```go
// Counter distinguishes unavailable usage from an observed zero.
type Counter struct {
    Value int64
    Known bool
}
```

## 9. Приёмка и проверка

| ID | Проверяемое условие |
|---|---|
| ST-01 | Все пять провайдеров: обычный и tool paths; отсутствие/null/ноль/invalid/overflow, cache/reasoning без двойного счёта |
| ST-02 | Query с несколькими tool turns и safety review: каждый call ровно один раз; review входит в total, не заменяет context |
| ST-03 | Невалидный options JSON после valid usage, failure/cancel, interrupted SSE и final usage-only chunk сохраняют реально известный расход |
| ST-04 | Повтор snapshots и идемпотентная финализация не удваивают tokens; conflicting finalize не перезаписывает запись; transport retry раскрывает ограничение |
| ST-05 | Старые диалоги, no usage, partial/zero usage и no DB дают разные корректные состояния; backfill-выдумок нет |
| ST-06 | No-history не создаёт/не открывает storage; сбой journal не ломает query и не повторяет внешнее действие; machine stdout чист |
| ST-07 | День/год/DST, midnight crossing, границы `[from,to)`, фильтры и стабильный порядок совпадают с контрактом |
| ST-08 | Session last выбирает последний диалог даже без usage; continuation не копирует расход; delete исключает связанный usage |
| ST-09 | Input snapshot не использует cumulative total или review; unknown limit не даёт процента; stale timestamp виден |
| ST-10 | Fixtures 120/80/40/20 колонок, Unicode/control characters, NO_COLOR, dumb и pipe; легенда и неполное покрытие видны без цвета |
| ST-11 | TUI keys, resize, q/Escape/Ctrl-C, refresh и восстановление терминала; headless interactive отклоняется без prompt |
| ST-12 | JSON schema/nullable fields/order/sums/empty arrays; parser conflicts и exit codes; ошибки чтения оставляют stdout пустым |
| ST-13 | Invalid LLM/embedding settings и запрещённый HTTP transport не мешают локальному stats/help; никакой сети/выполнения команд |
| ST-14 | Atomic additive migration, rollback failure, future schema/corruption/busy, concurrent query writer и stats reader; история не теряется |
| ST-15 | Built-in routing/registry reservation не ломает opaque argv других entities и доступ к существующему alias/script stats |
| ST-16 | Тексты en/ru синхронны; docs/help объясняют query-only scope, history privacy, unknown coverage и input-vs-consumption |

Unit fixtures синтетические, без реальных ключей/истории; HTTP — httptest, время и timezone контролируются. Тест сначала воспроизводит требуемое поведение; UI golden snapshots не заменяют тест чисел. Для concurrent collector/store применяется race detector. E2E собирает реальный бинарник, использует isolated HOME и проверяет stdout/stderr/exit/storage side effects.

Команды проверки **после реализации**, не утверждение об их запуске:

```bash
make test
make lint
make test-race
make test-e2e
make build
make clean
```

Во время итераций — целевые `go test ./internal/entities/history/...` и тесты новых stats-пакетов/CLI. Изменения поведения сопровождаются CHANGELOG, обеими README и релевантными HISTORY/ARCHITECTURE docs. Для данного черновика достаточно проверки Markdown links, fences, parity и whitespace; source-mutating `make lint` не запускается ради prose-only задачи.

## 10. Границы и согласование

**Всегда:** сохранять пользовательские изменения; проверять фактические SDK/paths; ограничивать сбор query_llm; различать unknown и zero; использовать существующий history lifecycle/privacy; проверять обе локали.

**Сначала согласовать:** этот draft и предложенные defaults; изменение provider/storage contracts и additive migration на следующем этапе; новые зависимости; включение cost, отдельного хранения при disabled history или дополнительных LLM workflows. Точный план не должен расширять утверждённый scope.

**Никогда:** оценивать точные tokens по длине текста; выдавать локальный счётчик за квоту аккаунта; повторять LLM ради записи usage; писать секреты/raw payload в journal; автоматически удалять данные; запускать команды из dashboard; менять safety policy; коммитить или мигрировать реальные данные только на основании написанного draft.

Вопросы для утверждения: принять ли static-by-default + explicit interactive; привязать ли retention к истории без independent statistics; оставить ли стоимость и approximate context breakdown за первой версией. Рекомендация — принять все три defaults. Числовые проценты контекста появятся только при достоверном источнике лимита; отсутствие такого источника не блокирует token/activity dashboard.

## 11. Референсы исследования

Источники — образцы UX/учёта, не код для копирования и не обязательство повторить весь продукт:

- [Claude Code commands](https://code.claude.com/docs/en/commands): stats/usage и context; реализация CLI не была доступна в проверенном публичном дереве.
- [Codex last/total usage](https://github.com/openai/codex/blob/6257fbc4bd5339144e6ffb3398a011917d883a2e/codex-rs/protocol/src/protocol.rs#L2617-L2653), [activity charts](https://github.com/openai/codex/blob/6257fbc4bd5339144e6ffb3398a011917d883a2e/codex-rs/tui/src/analytics/activity_chart.rs).
- [OpenCode stats presentation](https://github.com/anomalyco/opencode/blob/0b87612685dad0bb961d4086e650eb51ec4692cc/packages/cli/src/commands/handlers/stats.ts), [aggregation](https://github.com/anomalyco/opencode/blob/0b87612685dad0bb961d4086e650eb51ec4692cc/packages/core/src/session/stats.ts). Его activity calendar считает steps, не tokens.
- [Gemini CLI heatmap](https://github.com/google-gemini/gemini-cli/blob/4f343cabc7b59ab990d7411e920094108524abff/packages/cli/src/ui/components/HeatmapComponent.tsx).
- [Aider context tokens](https://github.com/Aider-AI/aider/blob/5dc9490bb35f9729ef2c95d00a19ccd30c26339c/aider/commands.py#L445-L551).
- [ccusage daily aggregation](https://github.com/ryoppippi/ccusage/blob/7ac5f2329fe1cd6bed5645867b18703ad86aaba5/rust/adapters/claude/src/daily.rs).
- [Anthropic prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), [OpenAI prompt caching](https://platform.openai.com/docs/guides/prompt-caching/best-practices), [Gemini usage metadata](https://ai.google.dev/api/generate-content#UsageMetadata).

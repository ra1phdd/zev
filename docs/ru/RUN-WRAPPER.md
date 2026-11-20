<div align="center">

[English](../en/RUN-WRAPPER.md) | **Русский**

</div>

# `zev run`: shell execution и recovery

`zev run '<command>'` — отдельная обёртка для запуска одной shell-команды. Команда выполняется внутри настроенного `$SHELL`; `zev run` не является retry loop или sandbox.

## Контракт разбора

Опции wrapper задаются до команды. После опций должен идти ровно один аргумент с shell-текстом; Zev не разбирает его заранее и передаёт как программу настроенному shell.

```bash
zev run 'git status --short'
zev run --timeout=30s --log-limit=65536 './build/check --format json'
```

Поддерживаемые опции:

| Опция | Контракт |
|---|---|
| `--on-failure=ask|none|analyze|fix` | Политика после ошибки; по умолчанию `ask` |
| `--timeout=DURATION` | Таймаут одной попытки от `0s` до `24h`; `0s` — без timeout wrapper |
| `--log-limit=BYTES` | Лимит общего захвата от 1 КиБ до 16 МиБ; по умолчанию 1 МиБ |
| `--session=ID` | Ровно 10 символов из локального алфавита session |
| `--proxy=URL` | Proxy для recovery/provider-запросов |
| `--no-interactive` | Отключить выбор recovery и уточняющие prompts |

Неизвестные опции, опции после `--`, отсутствие разделителя, пустой executable и пропущенное значение опции отклоняются до запуска дочернего процесса. Каждый вызов выполняет одну попытку; retry count, delay и retry exit-code не поддерживаются.

## Процесс и вывод

Zev передаёт stdout/stderr настроенного shell вызывающей стороне и одновременно сохраняет ограниченные хвосты потоков отдельно для recovery. Захваченные байты преобразуются в valid UTF-8, управляющие символы экранируются в failure log. Настроенный shell получает исходный текст команды и обычную process boundary wrapper. Recovery пишет в stderr и не заменяет исходный exit result.

`--timeout` ограничивает попытку через context cancellation. Очистка процесса может занять больше deadline; detached descendants и процессы, созданные внешними broker, находятся вне границы владения Zev. Это host execution, а не OS isolation.

## Политики после ошибки

- `none`: вывести ограниченный failure log и сохранить исходную ошибку.
- `ask`: в интерактивном терминале выбрать **Ничего не делать**, **Анализировать ошибку** или **Исправить команду**. Без интерактивного терминала recovery по умолчанию не выполняется.
- `analyze`: передать ограниченный diagnostic bundle провайдеру для объяснения.
- `fix`: при необходимости запросить уточнение и получить варианты исправленной команды.

Recovery никогда не запускает повторно упавшую команду и не выполняет предложенную замену. Исходная ошибка и exit status сохраняются. При отмене, timeout или неопределённом результате процесса recovery пропускается, чтобы не отправлять вводящий в заблуждение diagnostic.

Diagnostic bundle имеет schema version 1 и содержит literal command в display-quoted виде, текст ошибки, хвосты stdout/stderr и флаги усечения. Считайте его чувствительными данными: перед disclosure Zev предупреждает, потому что вывод может включать секреты. В запросе провайдеру bundle находится в недоверенном user-data поле; текст логов и output модели не становятся инструкциями для выполнения.

## Связанные сессии

Сгенерированная injection может добавить непрозрачный `--session` key. Он локально связывает canonical conversation ID и выбранный command ID в `~/.zev/run-sessions.json`. Registry приватен, ограничен по размеру, записывается атомарно и защищается кооперативной блокировкой. Session key не является credential и не даёт произвольный доступ к history: recovery проверяет связанную conversation и selected command. Явный `zev run` без session создаёт отдельный recovery query.

Registry — локальное удобство, не remote protocol. Неверные ID, malformed records, неканонические UUID-ссылки и слишком большой registry отклоняются fail-closed.

## Связь с command security

`zev run` выполняет shell-текст, поэтому намеренно предоставляет capability настроенного shell и не использует query `execute` authorization path или подтверждение `security.mode`. Пользователь явно вызвал wrapper; его технические границы сохраняются: ограниченный вывод, cancellation, фильтрация process handling там, где она применяется, и отсутствие автоматических повторов. Shell metacharacters обрабатывает настроенный shell.

Различие важно:

- query `inject` вставляет текст и оставляет запуск пользовательской shell;
- query `execute` — отдельный security-gated host delivery path;
- `zev run '<команда>'` выполняет одну shell-команду внутри Zev и сохраняет ограниченный failure context;
- recovery suggestions — только inert text.

## Проверка и ограничения

Сфокусированные тесты покрывают разбор опций, одну shell-команду, выполнение shell-синтаксиса, одну попытку, усечение вывода, cancellation, проверку session registry и связанной conversation. Они не доказывают безопасность каждой shell-команды или dialect, containment detached процессов и native Windows runtime. Запускайте `make test`, `make test-race` и применимый `make test-e2e` gate. См. [Безопасность](SECURITY.md), [CLI](CLI.md), [Конфигурацию](CONFIGURATION.md) и [Разработку](DEVELOPMENT.md).

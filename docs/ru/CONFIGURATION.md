<div align="center">

[English](../en/CONFIGURATION.md) | **Русский**

</div>

# Конфигурация

Zev читает конфигурацию из `~/.zevrc` — обычного файла формата `KEY=VALUE`, который
загружается через [godotenv](https://github.com/joho/godotenv) и разбирается
[caarlos0/env](https://github.com/caarlos0/env/v11).

Файл создаётся и обновляется командой `zev setup`:

```bash
zev setup              # интерактивный мастер
zev setup mode         # inject | execute
zev setup headers      # произвольные HTTP-заголовки
```

## Значения по умолчанию

Указанные ниже значения применяются в `config.DefaultConfig()`, если соответствующего
ключа нет в `~/.zevrc`.

| Ключ | По умолчанию |
|---|---|
| `LLM_PROVIDER` | `openai` |
| `OpenAIModel` | `gpt-5.6-luna` |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `GEMINI_BASE_URL` | `https://generativelanguage.googleapis.com` |
| `GeminiModel` | `gemini-3.1-flash-lite` |
| `ANTHROPIC_BASE_URL` | `https://api.anthropic.com/v1` |
| `AnthropicModel` | `claude-haiku-5.5` |
| `COMMAND_EXECUTION_MODE` | `inject` |
| `ScriptEditor` | `nano` |

## Переменные

### Выбор провайдера

| Переменная | Описание |
|---|---|
| `LLM_PROVIDER` | `openai`, `gemini`, `anthropic`, `ollama` или `opencode` |

### OpenAI / OpenCode

| Переменная | Описание |
|---|---|
| `OPENAI_API_KEY` | API-ключ. Также используется провайдером `opencode` |
| `OPENAI_MODEL` | Название модели |
| `OPENAI_BASE_URL` | Базовый URL — можно указать любой OpenAI-совместимый роутер |

### Gemini

| Переменная | Описание |
|---|---|
| `GEMINI_API_KEY` | API-ключ |
| `GEMINI_MODEL` | Название модели |
| `GEMINI_BASE_URL` | Базовый URL |

### Anthropic

| Переменная | Описание |
|---|---|
| `ANTHROPIC_API_KEY` | API-ключ |
| `ANTHROPIC_MODEL` | Название модели |
| `ANTHROPIC_BASE_URL` | Базовый URL |

### Ollama

| Переменная | Описание |
|---|---|
| `OLLAMA_BASE_URL` | Локальный эндпоинт Ollama, например `http://localhost:11434/v1` |
| `OLLAMA_MODEL` | Название модели |

### Поведение

| Переменная | Описание |
|---|---|
| `COMMAND_EXECUTION_MODE` | `inject` — команда подставляется в терминал для проверки; `execute` — выполняется сразу |
| `SCRIPT_EDITOR` | Редактор для `zev script create` |
| `HTTP_HEADERS` | Список пар `Key:Value` через запятую, добавляемых к каждому запросу. Регистр ключей сохраняется |

## Режим выполнения команд

`zev setup mode` определяет, что происходит после выбора подсказки:

- `inject` (по умолчанию) — команда попадает в терминал, вы её проверяете и жмёте Enter.
- `execute` — команда выполняется немедленно. Используйте только если полностью
  доверяете подсказкам; опасные команды помечаются предупреждением в любом режиме.

## Выбор редактора для `zev script create`

1. `$SCRIPT_EDITOR`
2. `$EDITOR`
3. первый доступный из `nano`, `vim`, `vi`

Если редактор не найден, Zev выведет путь к созданному файлу вместо ошибки.

## Файлы на диске

| Путь | Содержимое |
|---|---|
| `~/.zevrc` | Конфигурация, строки `KEY=VALUE` |
| `~/.zevhistory` | История запросов, JSONL, ротация на 100 записей |
| `~/.zev/scripts` | Метаданные скриптов, YAML |
| `~/.zev/script_files` | Тела скриптов |

`~/.zev/scripts` — это файл, а `~/.zev/script_files` — каталог: файл и каталог с
одинаковым именем не могут сосуществовать.

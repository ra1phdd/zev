<div align="center">
<h1>Zev</h1>
<p>Подсказки команд терминала на естественном языке — опишите задачу, получите команду</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat" alt="Платформы">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="Лицензия">
    <a href="https://github.com/ra1phdd/zev/actions/workflows/ci.yml"><img src="https://github.com/ra1phdd/zev/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

[English](README.md) | **Русский**
</div>

## Возможности

- Преобразование запросов на естественном языке в варианты команд терминала с пояснениями.
- Вставка выбранных подсказок в терминал для проверки без нажатия Enter.
- Сохранение часто используемых команд в виде алиасов с позиционными аргументами.
- Создание и запуск переиспользуемых Bash-скриптов.
- Поддержка OpenAI, Gemini, Anthropic, Ollama и OpenCode.

## Быстрый старт

Установите Zev по инструкциям ниже и настройте провайдера:

```sh
zev setup
zev "найти файлы, изменённые за последние 24 часа"
```

По умолчанию Zev вставляет выбранную команду в терминал для проверки, но не нажимает Enter. Чтобы явно указать режим вставки:

```sh
zev --inject "показать запущенные Docker-контейнеры"
```

## Установка

### Бинарные файлы

Скачайте последний релиз со страницы [GitHub Releases](https://github.com/ra1phdd/zev/releases) или установите в Linux/macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
```

В Windows выполните в PowerShell:

```powershell
irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.ps1 | iex
```

Скрипты установки обязательно проверяют SHA-256 контрольную сумму архива.

### Docker

Соберите собственный контейнерный образ вокруг бинарного файла и при необходимости подключите каталог конфигурации Zev для сохранения настроек провайдера. Интерактивная вставка команды зависит от терминала хоста и не предоставляется контейнером.

### Из исходников

Требуется версия Go, указанная в [`go.mod`](go.mod):

```sh
go install github.com/ra1phdd/zev/cmd/zev@latest
```

## Типичное использование

### Запрос на естественном языке и вставка команды

```sh
zev "найти файлы размером больше 100 МБ"
zev "показать текущую ветку Git"
```

Проверьте подсказку перед запуском. Zev вставляет команду, но не нажимает Enter.

### Алиас

Сохраните шаблон команды и вызывайте его по имени, передавая аргумент в конце:

```sh
zev alias add dps "docker service ps {{.arg1}} --no-trunc"
zev dps my-service
```

### Скрипт

Создайте именованный Bash-скрипт, отредактируйте его тело и вызывайте по имени:

```sh
zev script create deploy "Деплой приложения"
zev deploy
```

## Документация

- [Полный справочник CLI](docs/ru/CLI.md) — дерево команд, аргументы, все локальные и наследуемые флаги, значения по умолчанию, алиасы, скрытые флаги и shell completion.
- [Конфигурация](docs/ru/CONFIGURATION.md)
- [Безопасность](docs/ru/SECURITY.md) — реализованная policy, границы tools, web fetch и приватности.
- [История](docs/ru/HISTORY.md)
- [Алиасы и скрипты](docs/ru/ALIASES-SCRIPTS.md)
- [Обёртка `zev run`](docs/ru/RUN-WRAPPER.md)
- [Обновление](docs/ru/UPDATE.md)
- [Архитектура](docs/ru/ARCHITECTURE.md)
- [Разработка](docs/ru/DEVELOPMENT.md)
- [Участие в разработке](CONTRIBUTING.ru.md)

Справка: `zev --help` или `zev COMMAND --help`. Команды `zev help` нет; полный список команд и флагов приведён в справочнике CLI.

## Контрибуция

Приветствуем вклад — читайте [гайд по участию](CONTRIBUTING.ru.md).

## Лицензия

Проект лицензирован по MIT License — см. файл [LICENSE](LICENSE).

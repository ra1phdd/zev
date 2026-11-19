package cli

import (
	"fmt"
	"strings"

	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

const completionBashShell = "bash"

func localizeCommandHelp(root *cobra.Command) {
	if locale.Language() == "ru" {
		template := localizedUsageTemplate(root.UsageTemplate())
		forEachCommand(root, func(cmd *cobra.Command) { cmd.SetUsageTemplate(template) })
	}
	localizeHelpFlags(root)
	localizeCompletionHelp(root)
	localizeRunHelp(root)
}

func localizedUsageTemplate(template string) string {
	for _, heading := range []string{"Global Flags:", "Usage:", "Aliases:", "Examples:", "Available Commands:", "Additional Commands:", "Flags:", "Additional help topics:"} {
		template = strings.ReplaceAll(template, heading, locale.TextSource(heading))
	}
	template = strings.ReplaceAll(template, "(default ", "(по умолчанию ")
	return strings.ReplaceAll(template, `Use "{{.CommandPath}} [command] --help" for more information about a command.`, locale.Text(locale.KeyQueryCommandpathCommandHelpMoreInformationAboutCommand))
}

func forEachCommand(root *cobra.Command, visit func(*cobra.Command)) {
	visit(root)
	for _, child := range root.Commands() {
		forEachCommand(child, visit)
	}
}

func localizeHelpFlags(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	if flag := cmd.Flags().Lookup("help"); flag != nil {
		flag.Usage = locale.Text(locale.KeyQueryShowHelpCommand)
	}
	for _, child := range cmd.Commands() {
		localizeHelpFlags(child)
	}
}

func localizeCompletionHelp(root *cobra.Command) {
	if locale.Language() != "ru" {
		return
	}
	forEachCommand(root, func(cmd *cobra.Command) {
		if cmd.Name() == "completion" {
			cmd.Short = "Создать скрипт автодополнения для указанной оболочки"
			cmd.Long = "Создать скрипт автодополнения для Zev для указанной оболочки.\nПодробности приведены в справке каждой подкоманды."
			return
		}
		if cmd.Parent() == nil || cmd.Parent().Name() != "completion" {
			return
		}
		switch cmd.Name() {
		case completionBashShell:
			cmd.Short = "Создать скрипт автодополнения для bash"
		case "fish":
			cmd.Short = "Создать скрипт автодополнения для fish"
		case "powershell":
			cmd.Short = "Создать скрипт автодополнения для PowerShell"
		case "zsh":
			cmd.Short = "Создать скрипт автодополнения для zsh"
		}
	})
}

func localizeRunHelp(root *cobra.Command) {
	forEachCommand(root, func(cmd *cobra.Command) {
		if cmd.Name() != "run" || cmd.RunE == nil {
			return
		}
		original := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if locale.Language() == "ru" && isRunHelp(args) {
				return writeRussianRunHelp(cmd)
			}
			return original(cmd, args)
		}
	})
}

func writeRussianRunHelp(cmd *cobra.Command) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), `Использование: zev run [параметры] -- команда [аргументы...]

Параметры:
  --on-failure=ask|none|analyze|fix
                               Политика при ошибке; analyze/fix отправляют вывод провайдеру
  --timeout=D                  Тайм-аут одной попытки (0–24 ч)
  --log-limit=BYTES            Максимальный объём stdout/stderr на попытку (1 КиБ–16 МиБ)
  --session=ID                 10-значный ключ связанного диалога/команды
  --proxy=URL                  Направлять запросы восстановления через HTTP(S)- или SOCKS5-прокси
  --no-interactive             Отключить запросы подтверждения восстановления

Восстановление отправляет захваченный вывод провайдеру; вывод может содержать секреты. Вывод никогда не выполняется автоматически. Аргументы после -- передаются буквально, без разбора shell.`)
	if err != nil {
		return fmt.Errorf("write Russian run help: %w", err)
	}
	return nil
}

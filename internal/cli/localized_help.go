package cli

import (
	"strings"

	"github.com/ra1phdd/zev/locale"
	"github.com/spf13/cobra"
)

func localizeCommandHelp(root *cobra.Command) {
	if locale.Language() != "ru" {
		return
	}
	template := root.UsageTemplate()
	for _, heading := range []string{"Global Flags:", "Usage:", "Aliases:", "Examples:", "Available Commands:", "Additional Commands:", "Flags:", "Additional help topics:"} {
		template = strings.ReplaceAll(template, heading, locale.TextSource(heading))
	}
	template = strings.ReplaceAll(template, `Use "{{.CommandPath}} [command] --help" for more information about a command.`, locale.Text(locale.KeyQueryCommandpathCommandHelpMoreInformationAboutCommand))
	root.SetUsageTemplate(template)
	root.InitDefaultHelpCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "help" {
			cmd.Short = locale.Text(locale.KeyQueryHelpAboutAnyCommand)
			cmd.Long = locale.Text(locale.KeyQueryHelpProvidesHelpAnyCommandApplication)
		}
	}
	localizeHelpFlags(root)
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

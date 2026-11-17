package main

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
		template = strings.ReplaceAll(template, heading, locale.Text(heading))
	}
	template = strings.ReplaceAll(template, `Use "{{.CommandPath}} [command] --help" for more information about a command.`, locale.Text(`Use "{{.CommandPath}} [command] --help" for more information about a command.`))
	root.SetUsageTemplate(template)
	root.InitDefaultHelpCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "help" {
			cmd.Short = locale.Text("Help about any command")
			cmd.Long = locale.Text("Help provides help for any command in the application.")
		}
	}
	localizeHelpFlags(root)
}

func localizeHelpFlags(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	if flag := cmd.Flags().Lookup("help"); flag != nil {
		flag.Usage = locale.Text("Show help for this command")
	}
	for _, child := range cmd.Commands() {
		localizeHelpFlags(child)
	}
}

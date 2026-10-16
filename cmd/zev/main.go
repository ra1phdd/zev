package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Версия приложения (устанавливается при сборке).
var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "zev [query]",
		Short:   "Natural language terminal command suggestions",
		Long:    "Zev helps you remember terminal commands by describing what you want to do in natural language.",
		Version: version,
		Args:    cobra.ArbitraryArgs,
		RunE:    runQuery,
		// Отключаем автовывод справки при ошибках
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Добавляем подкоманды
	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newHistoryCmd())
	rootCmd.AddCommand(newSetupCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

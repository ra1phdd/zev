// Package zev embeds the repository's localization catalogs for the internal locale package.
package zev

import "embed"

// LocaleFiles contains the TOML catalogs used by the CLI and TUI.
//
//go:embed locale/*.toml
var LocaleFiles embed.FS

package domain

// Alias представляет алиас команды с шаблоном подстановки аргументов.
type Alias struct {
	Name     string
	Template string
}

// Script представляет bash-скрипт с метаданными.
type Script struct {
	Name        string
	ScriptFile  string
	Description string
}

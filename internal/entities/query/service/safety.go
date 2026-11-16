package service

import (
	"strings"
	"unicode"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	safetyRedirectOperator = "redirect"
	safetyInputOperator    = "input"
	safetyDeleteCommand    = "del"
)

// CommandAssessment is the local safety assessment of a command string.
type CommandAssessment struct {
	Dangerous   bool
	Explanation string
}

// ValidateCommand performs a bounded lexical check without executing the command.
// It is a conservative heuristic, not a shell parser or a sandbox guarantee.
func ValidateCommand(command string) CommandAssessment {
	tokens, syntax := tokenizeCommand(command)
	segments := splitCommandSegments(tokens)

	for _, segment := range segments {
		if assessment, ok := assessCommandSegment(segment); ok {
			return assessment
		}
	}

	if syntax.commandSubstitution {
		return dangerousAssessment("Command substitution can execute a nested command.")
	}
	if syntax.remoteShellPipeline {
		return dangerousAssessment("Remote content is being piped directly to a shell.")
	}
	if syntax.chain {
		return dangerousAssessment("Shell chaining can execute multiple commands.")
	}
	if syntax.outputRedirect {
		return dangerousAssessment("Output redirection can overwrite files or devices.")
	}
	return CommandAssessment{}
}

// ApplyDangerousValidation upgrades provider responses using the local heuristic.
// Existing provider warnings and explanations are never changed.
func ApplyDangerousValidation(response *domain.OptionsResponse) {
	if response == nil {
		return
	}

	for i := range response.Commands {
		command := &response.Commands[i]
		if command.IsDangerous {
			continue
		}
		assessment := ValidateCommand(command.Command)
		if assessment.Dangerous {
			command.IsDangerous = true
			command.DangerousExplanation = assessment.Explanation
		}
	}
}

type safetyToken struct {
	operator bool
	text     string
}

type syntaxAssessment struct {
	chain               bool
	commandSubstitution bool
	outputRedirect      bool
	remoteShellPipeline bool
}

type commandTokenizer struct {
	tokens        []safetyToken
	syntax        syntaxAssessment
	word          strings.Builder
	inSingleQuote bool
	inDoubleQuote bool
	wordStarted   bool
}

func (t *commandTokenizer) flushWord() {
	if !t.wordStarted {
		return
	}
	t.tokens = append(t.tokens, safetyToken{text: t.word.String()})
	t.word.Reset()
	t.wordStarted = false
}

func (t *commandTokenizer) appendWord(r rune) {
	t.word.WriteRune(r)
	t.wordStarted = true
}

func (t *commandTokenizer) consumeSingle(r rune) {
	if r == '\'' {
		t.inSingleQuote = false
		return
	}
	t.appendWord(r)
}

func (t *commandTokenizer) consumeDouble(runes []rune, index *int) {
	switch runes[*index] {
	case '"':
		t.inDoubleQuote = false
	case '\\':
		if *index+1 < len(runes) {
			(*index)++
			t.appendWord(runes[*index])
		} else {
			t.appendWord(runes[*index])
		}
	case '$':
		if *index+1 < len(runes) && runes[*index+1] == '(' {
			t.syntax.commandSubstitution = true
		}
		t.appendWord(runes[*index])
	case '`':
		t.syntax.commandSubstitution = true
		t.appendWord(runes[*index])
	default:
		t.appendWord(runes[*index])
	}
}

func (t *commandTokenizer) consumeUnquoted(runes []rune, index *int) {
	r := runes[*index]
	switch r {
	case '\'', '"':
		t.wordStarted = true
		t.inSingleQuote = r == '\''
		t.inDoubleQuote = r == '"'
	case '\\':
		t.wordStarted = true
		if *index+1 < len(runes) {
			(*index)++
			t.appendWord(runes[*index])
		}
	case '$':
		if *index+1 < len(runes) && runes[*index+1] == '(' {
			t.syntax.commandSubstitution = true
		}
		t.appendWord(r)
	case '`':
		t.syntax.commandSubstitution = true
		t.appendWord(r)
	case '\n', '\r':
		t.addChainOperator("newline")
	case ' ', '	', '\f', '\v':
		t.flushWord()
	case ';':
		t.addChainOperator(";")
	case '&':
		t.consumeAmpersand(runes, index)
	case '|':
		t.consumePipe(runes, index)
	case '>':
		t.consumeRedirect(runes, index)
	case '<':
		t.consumeInput(runes, index)
	default:
		t.appendWord(r)
	}
}

func (t *commandTokenizer) addChainOperator(operator string) {
	t.flushWord()
	t.tokens = append(t.tokens, safetyToken{operator: true, text: operator})
	t.syntax.chain = true
}

func (t *commandTokenizer) consumeAmpersand(runes []rune, index *int) {
	t.flushWord()
	switch {
	case *index+1 < len(runes) && runes[*index+1] == '>':
		(*index)++
		t.addRedirectOperator()
	case *index+1 < len(runes) && runes[*index+1] == '&':
		(*index)++
		t.addChainOperator("&&")
	default:
		t.addChainOperator("&")
	}
}

func (t *commandTokenizer) consumePipe(runes []rune, index *int) {
	t.flushWord()
	if *index+1 < len(runes) && runes[*index+1] == '|' {
		(*index)++
		t.addChainOperator("||")
		return
	}
	t.addChainOperator("|")
}

func (t *commandTokenizer) consumeRedirect(runes []rune, index *int) {
	t.flushWord()
	if *index+1 < len(runes) && (runes[*index+1] == '>' || runes[*index+1] == '|' || runes[*index+1] == '&') {
		(*index)++
	}
	t.addRedirectOperator()
}

func (t *commandTokenizer) addRedirectOperator() {
	t.tokens = append(t.tokens, safetyToken{operator: true, text: safetyRedirectOperator})
	t.syntax.outputRedirect = true
}

func (t *commandTokenizer) consumeInput(runes []rune, index *int) {
	t.flushWord()
	if *index+1 < len(runes) && (runes[*index+1] == '<' || runes[*index+1] == '>') {
		(*index)++
	}
	t.tokens = append(t.tokens, safetyToken{operator: true, text: safetyInputOperator})
}

func tokenizeCommand(command string) ([]safetyToken, syntaxAssessment) {
	tokenizer := commandTokenizer{}
	runes := []rune(command)
	index := 0
	for index < len(runes) {
		switch {
		case tokenizer.inSingleQuote:
			tokenizer.consumeSingle(runes[index])
		case tokenizer.inDoubleQuote:
			tokenizer.consumeDouble(runes, &index)
		default:
			tokenizer.consumeUnquoted(runes, &index)
		}
		index++
	}
	tokenizer.flushWord()
	tokenizer.syntax.remoteShellPipeline = hasRemoteShellPipeline(tokenizer.tokens)
	return tokenizer.tokens, tokenizer.syntax
}

func splitCommandSegments(tokens []safetyToken) [][]string {
	var segments [][]string
	var words []string
	for _, token := range tokens {
		if token.operator {
			if len(words) > 0 {
				segments = append(segments, words)
				words = nil
			}
			continue
		}
		words = append(words, token.text)
	}
	if len(words) > 0 {
		segments = append(segments, words)
	}
	return segments
}

func assessCommandSegment(words []string) (CommandAssessment, bool) {
	commandIndex := commandStart(words)
	if commandIndex >= len(words) {
		return CommandAssessment{}, false
	}

	executable := executableName(words[commandIndex])
	args := words[commandIndex+1:]
	if strings.HasPrefix(executable, "mkfs.") {
		return dangerousAssessment("Filesystem formatting can destroy data on a disk."), true
	}
	switch executable {
	case "rm", "rmdir", safetyDeleteCommand, "erase", "rd", "remove-item":
		return dangerousAssessment("File or directory removal can cause data loss."), true
	case "sudo", "doas":
		return dangerousAssessment("Elevated privileges can affect protected system resources."), true
	case "dd":
		return dangerousAssessment("Raw disk copying can destroy data or overwrite a device."), true
	case "chmod":
		if insecureChmod(args) {
			return dangerousAssessment("Insecure permission changes can expose files or weaken security."), true
		}
	case "mkfs", "newfs", "format", "format-volume":
		return dangerousAssessment("Filesystem formatting can destroy data on a disk."), true
	case "docker":
		if dockerDestructive(args) {
			return dangerousAssessment("Destructive Docker operations can remove containers, images, volumes, or data."), true
		}
	case "kubectl", "oc":
		if kubectlDestructive(args) {
			return dangerousAssessment("Destructive cluster operations can remove or disrupt workloads."), true
		}
	case "find":
		if findDestructive(args) {
			return dangerousAssessment("This find operation can delete files through an action expression."), true
		}
	case "git":
		if gitDestructive(args) {
			return dangerousAssessment("This Git operation can delete work or rewrite repository history."), true
		}
	case "cat", "head", "tail", "less", "more", "type":
		if readsSensitiveFile(args) {
			return dangerousAssessment("Reading this path can expose sensitive system data."), true
		}
	}

	if shellInterpreter(executable, args) {
		return dangerousAssessment("Shell interpreter execution can run arbitrary commands."), true
	}
	return CommandAssessment{}, false
}

func commandStart(words []string) int {
	index := 0
	for index < len(words) {
		if isAssignment(words[index]) {
			index++
			continue
		}
		executable := executableName(words[index])
		switch executable {
		case "env", "command", "exec":
			index++
			for index < len(words) {
				if isAssignment(words[index]) || words[index] == "--" {
					index++
					continue
				}
				if executable == "env" && strings.HasPrefix(words[index], "-") {
					index++
					continue
				}
				break
			}
		default:
			return index
		}
	}
	return len(words)
}

func isAssignment(word string) bool {
	index := strings.IndexByte(word, '=')
	if index <= 0 {
		return false
	}
	for _, r := range word[:index] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func executableName(word string) string {
	word = strings.TrimSpace(strings.Trim(word, "'\""))
	word = strings.ReplaceAll(word, "\\", "/")
	if slash := strings.LastIndexByte(word, '/'); slash >= 0 {
		word = word[slash+1:]
	}
	word = strings.ToLower(word)
	word = strings.TrimSuffix(word, ".exe")
	return word
}

func gitDestructive(args []string) bool {
	subcommand := ""
	for index := 0; index < len(args); index++ {
		lower := strings.ToLower(args[index])
		if subcommand == "" && strings.HasPrefix(lower, "-") {
			index = skipGitGlobalOption(args, index)
			continue
		}
		if subcommand == "" {
			subcommand = lower
		}
		if gitSubcommandDangerous(subcommand, lower) {
			return true
		}
	}
	return false
}

func skipGitGlobalOption(args []string, index int) int {
	if gitGlobalOption(strings.ToLower(args[index])) && index+1 < len(args) {
		return index + 1
	}
	return index
}

func gitGlobalOption(argument string) bool {
	switch argument {
	case "-c", "--git-dir", "--work-tree":
		return true
	default:
		return false
	}
}

func gitSubcommandDangerous(subcommand, argument string) bool {
	switch subcommand {
	case "reset":
		return argument == "--hard"
	case "clean", "push":
		return argument == "--force" || shortFlag(argument, 'f')
	default:
		return false
	}
}

func shortFlag(value string, flag rune) bool {
	return strings.HasPrefix(value, "-") && !strings.HasPrefix(value, "--") && strings.ContainsRune(strings.TrimLeft(value, "-"), flag)
}

func insecureChmod(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "-") {
			continue
		}
		if lower == "777" || lower == "666" || lower == "0777" || lower == "0666" {
			return true
		}
		for _, marker := range []string{"a+rwx", "a+w", "o+w", "g+w", "+w", "u+s", "g+s"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

func dockerDestructive(args []string) bool {
	for i, arg := range args {
		lower := strings.ToLower(arg)
		switch lower {
		case "prune", "rm", "rmi", "kill", "stop":
			return true
		case "down":
			if i > 0 && strings.ToLower(args[i-1]) == "compose" {
				return true
			}
		}
	}
	return false
}

func kubectlDestructive(args []string) bool {
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "delete", safetyDeleteCommand, "drain", "cordon", "uncordon":
			return true
		}
	}
	return false
}

func findDestructive(args []string) bool {
	for i, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-delete" || lower == "--delete" {
			return true
		}
		if lower != "-exec" && lower != "-execdir" {
			continue
		}
		for _, action := range args[i+1:] {
			if action == ";" || action == "+" {
				break
			}
			if isRemovalExecutable(action) {
				return true
			}
		}
	}
	return false
}

func isRemovalExecutable(word string) bool {
	switch executableName(word) {
	case "rm", "rmdir", safetyDeleteCommand, "erase", "rd", "remove-item":
		return true
	default:
		return false
	}
}

func readsSensitiveFile(args []string) bool {
	for _, arg := range args {
		path := strings.ToLower(strings.ReplaceAll(arg, "\\", "/"))
		if strings.HasSuffix(path, "/etc/shadow") || strings.HasSuffix(path, "/etc/gshadow") || strings.HasSuffix(path, "/sam") {
			return true
		}
	}
	return false
}

func shellInterpreter(executable string, args []string) bool {
	isShell := executable == "sh" || executable == "bash" || executable == "dash" || executable == "zsh" || executable == "ksh" || executable == "fish" || executable == "pwsh" || executable == "powershell" || executable == "cmd"
	if !isShell {
		return false
	}
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "-c", "/c", "-command", "-encodedcommand":
			return true
		}
	}
	return false
}

func hasRemoteShellPipeline(tokens []safetyToken) bool {
	for i, token := range tokens {
		if !token.operator || token.text != "|" {
			continue
		}
		left := pipelineCommandName(tokens, i, -1)
		right := pipelineCommandName(tokens, i, 1)
		if (left == "curl" || left == "wget" || left == "invoke-webrequest") && isShellName(right) {
			return true
		}
	}
	return false
}

func pipelineCommandName(tokens []safetyToken, pipeIndex, direction int) string {
	if direction < 0 {
		start := pipeIndex - 1
		for start >= 0 && !isCommandBoundary(tokens[start]) {
			start--
		}
		for index := start + 1; index < pipeIndex; index++ {
			if !tokens[index].operator {
				return executableName(tokens[index].text)
			}
		}
		return ""
	}

	for index := pipeIndex + 1; index < len(tokens) && !isCommandBoundary(tokens[index]); index++ {
		if !tokens[index].operator {
			return executableName(tokens[index].text)
		}
	}
	return ""
}

func isCommandBoundary(token safetyToken) bool {
	return token.operator && token.text != safetyRedirectOperator && token.text != "input"
}

func isShellName(name string) bool {
	switch name {
	case "sh", "bash", "dash", "zsh", "ksh", "fish", "pwsh", "powershell", "cmd":
		return true
	default:
		return false
	}
}

func dangerousAssessment(explanation string) CommandAssessment {
	return CommandAssessment{Dangerous: true, Explanation: explanation}
}

package security

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/ra1phdd/zev/internal/core/domain"
)

const (
	safetyRedirectOperator           = "redirect"
	safetyDescriptorRedirectOperator = "descriptor-redirect"
	safetyInputOperator              = "input"
	safetyDeleteCommand              = "del"
)

// CommandAssessment is the local safety assessment of a command string.
type CommandAssessment struct {
	Dangerous   bool
	Explanation string
	Known       bool
}

// CommandOverrides adjusts exact command strings without changing structural checks.
type CommandOverrides struct {
	Add    []string
	Remove []string
}

// ValidateCommand analyzes command text using the POSIX sh grammar.
func ValidateCommand(command string) CommandAssessment {
	return ValidateCommandForShell(command, "sh")
}

// ValidateCommandForShell selects the parser from the resolved interpreter.
// Unsupported interpreters remain unknown instead of being treated as POSIX.
func ValidateCommandForShell(command, interpreter string) CommandAssessment {
	return ValidateCommandWithOverrides(command, interpreter, CommandOverrides{})
}

// ValidateCommandWithOverrides applies exact command overrides after structural analysis.
//
//nolint:gocognit // The bounded analyzer preserves conservative checks in one path.
func ValidateCommandWithOverrides(command, interpreter string, overrides CommandOverrides) CommandAssessment {
	if slices.Contains(overrides.Add, command) {
		return dangerousAssessment("Command is marked dangerous by user configuration.")
	}
	const (
		zsh = safetyZshCommand
		ksh = safetyKshCommand
	)
	if resolved, err := filepath.EvalSymlinks(interpreter); err == nil {
		interpreter = resolved
	}
	dialect := ShellDialect("unsupported")
	switch strings.ToLower(filepath.Base(interpreter)) {
	case "sh", safetyDashCommand, ksh:
		dialect = ShellPOSIX
	case safetyBashCommand:
		dialect = ShellBash
	case zsh:
		dialect = ShellZsh
	}
	structural := AnalyzeShell(command, dialect)
	if structural.Dangerous {
		if slices.Contains(overrides.Remove, command) && overridableCommandOnly(command, structural.Explanation) {
			return CommandAssessment{Known: true}
		}
		return CommandAssessment{Dangerous: true, Known: structural.Known, Explanation: structural.Explanation}
	}
	if !structural.Known {
		return CommandAssessment{}
	}
	tokens, syntax := tokenizeCommand(command)
	segments := splitCommandSegments(tokens)

	for _, segment := range segments {
		if assessment, ok := assessCommandSegment(segment); ok {
			if slices.Contains(overrides.Remove, command) && len(segments) == 1 && overridableDanger(assessment.Explanation) {
				return CommandAssessment{Known: true}
			}
			return assessment
		}
	}

	if syntax.commandSubstitution {
		nested := assessCommandSubstitutions(command)
		if nested.Dangerous {
			return nested
		}
		if !nested.Known || !allSegmentsKnownWithSubstitutions(segments) {
			return CommandAssessment{}
		}
	}
	if syntax.shellPipeline {
		return dangerousAssessment("Piping content to a shell can execute arbitrary commands.")
	}
	if syntax.ambiguousExpansion {
		return CommandAssessment{}
	}
	if syntax.outputRedirect {
		if redirectTargetsAreNull(tokens) && allSegmentsKnown(segments) {
			return CommandAssessment{Known: true}
		}
		return dangerousAssessment("Output redirection can overwrite files or devices.")
	}
	return CommandAssessment{Known: allSegmentsKnown(segments)}
}

// ApplyDangerousValidation upgrades provider responses using the local structural analyzer.
// Existing provider warnings and explanations are never changed.
func ApplyDangerousValidation(response *domain.OptionsResponse, overrides ...CommandOverrides) {
	if response == nil {
		return
	}
	var commandOverrides CommandOverrides
	if len(overrides) > 0 {
		commandOverrides = overrides[0]
	}

	for i := range response.Commands {
		command := &response.Commands[i]
		if command.IsDangerous && !command.LocallyAssessedDangerous {
			continue
		}
		if command.LocallyAssessedDangerous {
			command.IsDangerous = false
			command.DangerousExplanation = ""
			command.LocallyAssessedDangerous = false
		}
		assessment := ValidateCommandWithOverrides(command.Command, "sh", commandOverrides)
		if assessment.Dangerous {
			command.IsDangerous = true
			command.DangerousExplanation = assessment.Explanation
			command.LocallyAssessedDangerous = true
		}
	}
}

type safetyToken struct {
	operator bool
	text     string
}

type syntaxAssessment struct {
	commandSubstitution bool
	ambiguousExpansion  bool
	outputRedirect      bool
	shellPipeline       bool
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
	case '(':
		if *index > 0 && runes[*index-1] == '$' {
			t.appendWord(r)
			return
		}
		t.syntax.ambiguousExpansion = true
		t.appendWord(r)
	case '{', '}', '*', '?', '[', ']':
		t.syntax.ambiguousExpansion = true
		t.appendWord(r)
	default:
		t.appendWord(r)
	}
}

func (t *commandTokenizer) addChainOperator(operator string) {
	t.flushWord()
	t.tokens = append(t.tokens, safetyToken{operator: true, text: operator})
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
	if *index+1 < len(runes) && runes[*index+1] == '&' {
		(*index)++
		t.addDescriptorRedirectOperator()
		return
	}
	if *index+1 < len(runes) && (runes[*index+1] == '>' || runes[*index+1] == '|') {
		(*index)++
	}
	t.addRedirectOperator()
}

func (t *commandTokenizer) addRedirectOperator() {
	t.tokens = append(t.tokens, safetyToken{operator: true, text: safetyRedirectOperator})
	t.syntax.outputRedirect = true
}

func (t *commandTokenizer) addDescriptorRedirectOperator() {
	t.tokens = append(t.tokens, safetyToken{operator: true, text: safetyDescriptorRedirectOperator})
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
	tokenizer.syntax.shellPipeline = hasShellPipeline(tokenizer.tokens)
	return tokenizer.tokens, tokenizer.syntax
}

func splitCommandSegments(tokens []safetyToken) [][]string {
	var segments [][]string
	var words []string
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token.operator {
			if len(words) > 0 {
				segments = append(segments, words)
				words = nil
			}
			if (token.text == safetyRedirectOperator || token.text == safetyDescriptorRedirectOperator) && index+1 < len(tokens) && !tokens[index+1].operator {
				index++
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

func redirectTargetsAreNull(tokens []safetyToken) bool {
	found := false
	for index, token := range tokens {
		if !token.operator || token.text != safetyRedirectOperator {
			if token.text != safetyDescriptorRedirectOperator {
				continue
			}
			if index+1 >= len(tokens) || tokens[index+1].operator || !fileDescriptor(tokens[index+1].text) {
				return false
			}
			found = true
			continue
		}
		found = true
		if index+1 >= len(tokens) || tokens[index+1].operator || tokens[index+1].text != "/dev/null" {
			return false
		}
	}
	return found
}

func fileDescriptor(value string) bool {
	return value == "-" || value != "" && strings.Trim(value, "0123456789") == ""
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
	case "rm", safetyRmdirCommand, safetyDeleteCommand, safetyEraseCommand, "rd", safetyRemoveItemCommand:
		if isHelpOnly(args) {
			return CommandAssessment{}, true
		}
		return dangerousAssessment("File or directory removal can cause data loss."), true
	case safetySudoCommand, safetyDoasCommand:
		return dangerousAssessment("Elevated privileges can affect protected system resources."), true
	case "dd":
		return dangerousAssessment("Raw disk copying can destroy data or overwrite a device."), true
	case safetyChmodCommand:
		if insecureChmod(args) {
			return dangerousAssessment("Insecure permission changes can expose files or weaken security."), true
		}
	case safetyMkfsCommand, safetyNewfsCommand, safetyFormatCommand, safetyFormatVolumeCommand:
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
			return dangerousAssessment("This find action can execute a dangerous command or delete files."), true
		}
	case "git":
		if gitDestructive(args) {
			return dangerousAssessment("This Git operation can delete work or rewrite repository history."), true
		}
	case safetyCatCommand, safetyHeadCommand, safetyTailCommand, safetyLessCommand, safetyMoreCommand, safetyTypeCommand:
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
		case safetyEnvCommand, safetyCommandCommand, safetyExecCommand:
			index++
			for index < len(words) {
				if isAssignment(words[index]) || words[index] == "--" {
					index++
					continue
				}
				if executable == safetyEnvCommand && strings.HasPrefix(words[index], "-") {
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
	for i := 0; i < len(args); i++ {
		lower := strings.ToLower(args[i])
		if lower == "-delete" || lower == "--delete" {
			return true
		}
		if lower != "-exec" && lower != "-execdir" {
			continue
		}
		end := i + 1
		for end < len(args) && args[end] != ";" && args[end] != "+" {
			end++
		}
		if _, dangerous := assessCommandSegment(args[i+1 : end]); dangerous {
			return true
		}
		i = end
	}
	return false
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
	isShell := executable == "sh" || executable == safetyBashCommand || executable == safetyDashCommand || executable == safetyZshCommand || executable == safetyKshCommand || executable == safetyFishCommand || executable == safetyPwshCommand || executable == safetyPowerShellCommand || executable == safetyCmdCommand
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

func hasShellPipeline(tokens []safetyToken) bool {
	for i, token := range tokens {
		if !token.operator || token.text != "|" {
			continue
		}
		right := pipelineCommandName(tokens, i)
		if isShellName(right) {
			return true
		}
	}
	return false
}

func pipelineCommandName(tokens []safetyToken, pipeIndex int) string {
	var words []string
	for index := pipeIndex + 1; index < len(tokens) && !isCommandBoundary(tokens[index]); index++ {
		if !tokens[index].operator {
			words = append(words, tokens[index].text)
		}
	}
	start := commandStart(words)
	if start < len(words) {
		return executableName(words[start])
	}
	return ""
}

func isCommandBoundary(token safetyToken) bool {
	return token.operator && token.text != safetyRedirectOperator && token.text != "input"
}

func isShellName(name string) bool {
	switch name {
	case "sh", safetyBashCommand, safetyDashCommand, safetyZshCommand, safetyKshCommand, safetyFishCommand, safetyPwshCommand, safetyPowerShellCommand, safetyCmdCommand:
		return true
	default:
		return false
	}
}

func overridableCommandOnly(command, explanation string) bool {
	if !overridableDanger(explanation) {
		return false
	}
	tokens, _ := tokenizeCommand(command)
	segments := splitCommandSegments(tokens)
	if len(segments) != 1 {
		return false
	}
	assessment, ok := assessCommandSegment(segments[0])
	return ok && assessment.Dangerous && assessment.Explanation == explanation
}

const securityPermissionChangeExplanation = "Permission or ownership changes can weaken security."

func overridableDanger(explanation string) bool {
	switch explanation {
	case "File or directory removal can cause data loss.",
		"Filesystem or raw-device operations can destroy data.",
		"Elevated privileges can affect protected system resources.",
		securityPermissionChangeExplanation:
		return true
	default:
		return false
	}
}

func dangerousAssessment(explanation string) CommandAssessment {
	return CommandAssessment{Dangerous: true, Explanation: explanation, Known: true}
}

func isHelpOnly(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "--version" || args[0] == "-h")
}

func allSegmentsKnownWithSubstitutions(segments [][]string) bool {
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		index := commandStart(segment)
		if index >= len(segment) || !knownReadOnlyCommand(executableName(segment[index])) {
			return false
		}
		for _, argument := range segment[index+1:] {
			if strings.ContainsAny(argument, "$`") {
				continue
			}
			if strings.HasPrefix(argument, "~") || sensitivePath(argument) {
				return false
			}
		}
	}
	return true
}

func assessCommandSubstitutions(command string) CommandAssessment {
	if strings.Count(command, "$(") > 8 || strings.Count(command, "`") > 16 {
		return CommandAssessment{}
	}
	runes := []rune(command)
	for index := 0; index < len(runes); index++ {
		if runes[index] == '\'' {
			index = skipSingleQuoted(runes, index)
			continue
		}
		if assessment, end, found := assessBacktickSubstitution(runes, index); found {
			if assessment.Dangerous || !assessment.Known {
				return assessment
			}
			index = end
			continue
		}
		if assessment, end, found := assessDollarSubstitution(runes, index); found {
			if assessment.Dangerous || !assessment.Known {
				return assessment
			}
			index = end
		}
	}
	return CommandAssessment{Known: true}
}

func assessBacktickSubstitution(runes []rune, index int) (CommandAssessment, int, bool) {
	if runes[index] != '`' {
		return CommandAssessment{}, index, false
	}
	end := findBacktick(runes, index+1)
	if end < 0 {
		return CommandAssessment{}, index, true
	}
	return ValidateCommand(string(runes[index+1 : end])), end, true
}

func assessDollarSubstitution(runes []rune, index int) (CommandAssessment, int, bool) {
	if runes[index] != '$' || index+1 >= len(runes) || runes[index+1] != '(' {
		return CommandAssessment{}, index, false
	}
	end := findSubstitutionEnd(runes, index+1)
	if end < 0 {
		return CommandAssessment{}, index, true
	}
	return ValidateCommand(string(runes[index+2 : end])), end, true
}

func skipSingleQuoted(runes []rune, start int) int {
	for index := start + 1; index < len(runes); index++ {
		if runes[index] == '\'' {
			return index
		}
	}
	return len(runes)
}

func findBacktick(runes []rune, start int) int {
	for index := start; index < len(runes); index++ {
		if runes[index] == '\\' {
			index++
			continue
		}
		if runes[index] == '`' {
			return index
		}
	}
	return -1
}

func findSubstitutionEnd(runes []rune, start int) int {
	depth := 0
	for index := start; index < len(runes); index++ {
		switch runes[index] {
		case '\'':
			index = skipSingleQuoted(runes, index)
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		case '\\':
			index++
		}
	}
	return -1
}

func allSegmentsKnown(segments [][]string) bool {
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if !knownReadOnlySegment(segment) {
			return false
		}
	}
	return true
}

func knownReadOnlySegment(words []string) bool {
	index := commandStart(words)
	if index >= len(words) || !knownReadOnlyCommand(executableName(words[index])) {
		return false
	}
	for _, argument := range words[index+1:] {
		if strings.ContainsAny(argument, "$`") || strings.HasPrefix(argument, "~") || sensitivePath(argument) {
			return false
		}
	}
	return true
}

func knownReadOnlyCommand(command string) bool {
	switch command {
	case safetyCatCommand, "cut", "date", "du", "echo", safetyHeadCommand, safetyLessCommand, "ls", safetyMoreCommand, "printf", "pwd", "sort", "stat", safetyTailCommand, safetyTypeCommand, "uname", "wc", "which":
		return true
	default:
		return false
	}
}

func sensitivePath(value string) bool {
	path := strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
	for _, marker := range DefaultSensitivePathMarkers {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return strings.HasSuffix(path, ".pem") || strings.HasSuffix(path, ".key") || strings.HasSuffix(path, ".p12") || strings.HasSuffix(path, ".pfx")
}

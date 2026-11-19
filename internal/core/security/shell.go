package security

import (
	"slices"
	"strings"
	"unicode"
)

// ShellDialect identifies the grammar used to interpret shell text.
type ShellDialect string

const (
	// ShellPOSIX identifies the POSIX-compatible sh dialect.
	ShellPOSIX ShellDialect = "posix-sh"
	// ShellBash identifies the Bash dialect.
	ShellBash ShellDialect = "bash"
	// ShellZsh identifies the Z shell dialect.
	ShellZsh ShellDialect = "zsh"
)

const (
	maxShellNodes     = 16384
	maxShellRecursion = 32
)

// ShellAnalysis is the bounded structural result for one shell program.
// Unknown means that an unsupported construct, expansion, dialect, or budget
// boundary remains; it must not be converted into an automatic allow.
type ShellAnalysis struct {
	Dangerous   bool
	Known       bool
	Effects     []Effect
	Explanation string
	Nodes       int
}

type shellLexeme struct {
	text     string
	operator bool
	dynamic  bool
	quoted   bool
	redirect bool
	output   bool
	hereDoc  bool
}

type shellParser struct {
	dialect               ShellDialect
	nodes                 int
	depth                 int
	unknown               bool
	dangerousSubstitution string
}

// AnalyzeShell parses the supported POSIX sh, Bash, and Zsh subset without
// executing it. Fish, PowerShell, cmd, and unsupported syntax are unknown.
func AnalyzeShell(command string, dialect ShellDialect) ShellAnalysis {
	if dialect != ShellPOSIX && dialect != ShellBash && dialect != ShellZsh {
		return ShellAnalysis{}
	}
	parser := shellParser{dialect: dialect}
	return parser.program(command)
}

// AnalyzeArgv analyzes literal argv boundaries without reconstructing shell
// syntax. Shell metacharacters inside an argument remain ordinary bytes.
func AnalyzeArgv(argv []string) ShellAnalysis {
	if len(argv) == 0 {
		return ShellAnalysis{}
	}
	parser := shellParser{dialect: ShellPOSIX}
	words := make([]shellLexeme, 0, len(argv))
	for _, argument := range argv {
		words = append(words, shellLexeme{text: argument, quoted: true})
	}
	return parser.command(words)
}

func (p *shellParser) program(command string) ShellAnalysis {
	if p.depth > maxShellRecursion || len(command) > 64*1024 {
		return ShellAnalysis{}
	}
	lexemes, ok := p.lex(command)
	if !ok || len(lexemes) == 0 {
		return ShellAnalysis{}
	}
	if p.dangerousSubstitution != "" {
		return ShellAnalysis{Dangerous: true, Known: true, Effects: []Effect{EffectProcess}, Explanation: p.dangerousSubstitution, Nodes: 1}
	}
	var aggregate ShellAnalysis
	var words []shellLexeme
	for _, lexeme := range lexemes {
		if lexeme.operator && !lexeme.redirect && !lexeme.hereDoc {
			if lexeme.text == "(" && len(words) != 0 {
				p.unknown = true
				return ShellAnalysis{}
			}
			part := p.command(words)
			aggregate = mergeShellAnalysis(aggregate, part)
			words = nil
			if part.Dangerous {
				return aggregate
			}
			continue
		}
		words = append(words, lexeme)
	}
	aggregate = mergeShellAnalysis(aggregate, p.command(words))
	if aggregate.Dangerous {
		return aggregate
	}
	if p.unknown {
		aggregate.Known = false
	}
	if aggregate.Nodes == 0 {
		return ShellAnalysis{}
	}
	return aggregate
}

func mergeShellAnalysis(left, right ShellAnalysis) ShellAnalysis {
	left.Nodes += right.Nodes
	left.Dangerous = left.Dangerous || right.Dangerous
	left.Known = left.Known && right.Known
	if left.Nodes == right.Nodes {
		left.Known = right.Known
	}
	left.Effects = appendUniqueEffects(left.Effects, right.Effects...)
	if left.Explanation == "" {
		left.Explanation = right.Explanation
	}
	return left
}

func appendUniqueEffects(effects []Effect, additions ...Effect) []Effect {
	for _, addition := range additions {
		found := slices.Contains(effects, addition)
		if !found {
			effects = append(effects, addition)
		}
	}
	return effects
}

//nolint:gocognit // This is a bounded command grammar with conservative fallbacks.
func (p *shellParser) command(words []shellLexeme) ShellAnalysis {
	if len(words) == 0 {
		return ShellAnalysis{}
	}
	p.nodes++
	if p.nodes > maxShellNodes {
		p.unknown = true
		return ShellAnalysis{Nodes: 1}
	}
	analysis := ShellAnalysis{Known: true, Nodes: 1}
	var commandWords []string
	for index := 0; index < len(words); index++ {
		word := words[index]
		//nolint:nestif // Operator handling is kept together to preserve parser state.
		if word.operator {
			if word.redirect || word.output {
				if index+1 >= len(words) || words[index+1].operator {
					return ShellAnalysis{}
				}
				target := words[index+1]
				if target.dynamic {
					analysis.Known = false
				}
				if word.output {
					analysis.Effects = appendUniqueEffects(analysis.Effects, EffectWrite)
					if target.text != "/dev/null" && !shellFileDescriptor(target.text) {
						analysis.Dangerous = true
						analysis.Explanation = "Output redirection can overwrite files or devices."
					}
				}
				index++
			}
			continue
		}
		if word.dynamic {
			analysis.Known = false
		}
		commandWords = append(commandWords, word.text)
	}
	start := shellCommandStart(commandWords)
	if start >= len(commandWords) {
		return ShellAnalysis{}
	}
	executable := shellExecutable(commandWords[start])
	args := commandWords[start+1:]
	analysis.Effects = appendUniqueEffects(analysis.Effects, EffectProcess)
	switch executable {
	case "rm", "rmdir", "unlink", "del", "erase", "rd", "remove-item":
		if !shellHelpOnly(args) {
			analysis.Dangerous = true
			analysis.Explanation = "File or directory removal can cause data loss."
			analysis.Effects = appendUniqueEffects(analysis.Effects, EffectDelete)
		}
	case "dd", "mkfs", "mkfs.ext4", "newfs", "format", "format-volume":
		analysis.Dangerous = true
		analysis.Explanation = "Filesystem or raw-device operations can destroy data."
		analysis.Effects = appendUniqueEffects(analysis.Effects, EffectWrite)
	case "sudo", "doas":
		analysis.Dangerous = true
		analysis.Explanation = "Elevated privileges can affect protected system resources."
	case "chmod":
		if shellInsecureChmod(args) {
			analysis.Dangerous = true
			analysis.Explanation = "Permission or ownership changes can weaken security."
			analysis.Effects = appendUniqueEffects(analysis.Effects, EffectWrite)
		}
	case "chown", "setfacl":
		if !shellHelpOnly(args) {
			analysis.Dangerous = true
			analysis.Explanation = "Permission or ownership changes can weaken security."
			analysis.Effects = appendUniqueEffects(analysis.Effects, EffectWrite)
		}
	case "eval", "source", ".":
		analysis.Known = false
	case "sh", "bash", "dash", "zsh", "ksh":
		analysis = p.nestedInterpreter(analysis, args)
	case "env", "command", "exec":
		if len(args) == 0 {
			analysis.Known = false
		} else {
			nested := p.command([]shellLexeme{{text: strings.Join(args, " ")}})
			analysis = mergeShellAnalysis(analysis, nested)
		}
	default:
		if !knownShellCommand(executable) {
			analysis.Known = false
		}
	}
	return analysis
}

func (p *shellParser) nestedInterpreter(base ShellAnalysis, args []string) ShellAnalysis {
	for index, arg := range args {
		if arg != "-c" && arg != "-command" && arg != "--command" {
			continue
		}
		if index+1 >= len(args) {
			base.Known = false
			return base
		}
		p.depth++
		nested := p.program(args[index+1])
		p.depth--
		return mergeShellAnalysis(base, nested)
	}
	base.Dangerous = true
	base.Explanation = "Shell interpreter execution can run arbitrary commands."
	return base
}

//nolint:gocognit // Lexical states are explicit to keep unsupported syntax unknown.
func (p *shellParser) lex(command string) ([]shellLexeme, bool) {
	runes := []rune(command)
	var result []shellLexeme
	var word strings.Builder
	wordStarted := false
	quoted := false
	for index := 0; index < len(runes); index++ {
		r := runes[index]
		switch r {
		case '\'', '"':
			wordStarted = true
			quoted = true
			quote := r
			index++
			for index < len(runes) && runes[index] != quote {
				if quote == '"' && runes[index] == '$' {
					word.WriteRune(runes[index])
					wordStarted = true
				}
				word.WriteRune(runes[index])
				index++
			}
			if index >= len(runes) {
				return nil, false
			}
		case '\\':
			wordStarted = true
			if index+1 >= len(runes) {
				return nil, false
			}
			index++
			word.WriteRune(runes[index])
		case '$':
			wordStarted = true
			if index+1 < len(runes) && runes[index+1] == '(' {
				end := shellSubstitutionEnd(runes, index+1)
				if end < 0 {
					return nil, false
				}
				p.depth++
				nested := p.program(string(runes[index+2 : end]))
				p.depth--
				if nested.Dangerous {
					p.dangerousSubstitution = nested.Explanation
					return []shellLexeme{{text: "__dangerous_substitution__", dynamic: true}}, true
				}
				if !nested.Known {
					p.unknown = true
				}
				word.WriteString("substitution")
				index = end
				continue
			}
			word.WriteRune(r)
			if index+1 >= len(runes) || !unicode.IsDigit(runes[index+1]) {
				p.unknown = true
			}
		case '`':
			end := strings.IndexRune(string(runes[index+1:]), '`')
			if end < 0 {
				return nil, false
			}
			p.depth++
			nested := p.program(string(runes[index+1 : index+1+end]))
			p.depth--
			if nested.Dangerous {
				p.dangerousSubstitution = nested.Explanation
				return []shellLexeme{{text: "dangerous-substitution", dynamic: true}}, true
			}
			if !nested.Known {
				p.unknown = true
			}
			wordStarted = true
			word.WriteString("substitution")
			index += end + 1
		case ' ', '	', '\r':
			if wordStarted {
				result = append(result, shellLexeme{text: word.String(), dynamic: !quoted && p.wordDynamic(word.String()), quoted: quoted})
				word.Reset()
				wordStarted = false
				quoted = false
			}
		case '\n':
			if wordStarted {
				result = append(result, shellLexeme{text: word.String(), dynamic: !quoted && p.wordDynamic(word.String()), quoted: quoted})
				word.Reset()
				wordStarted = false
				quoted = false
			}
			result = append(result, shellLexeme{text: "newline", operator: true})
		case ';', '&', '|', '(', ')':
			if wordStarted {
				result = append(result, shellLexeme{text: word.String(), dynamic: !quoted && p.wordDynamic(word.String()), quoted: quoted})
				word.Reset()
				wordStarted = false
				quoted = false
			}
			operator := string(r)
			if index+1 < len(runes) && (runes[index+1] == r || (r == '&' && runes[index+1] == '>')) {
				index++
				operator += string(runes[index])
			}
			result = append(result, shellLexeme{text: operator, operator: true})
		case '>', '<':
			if wordStarted {
				result = append(result, shellLexeme{text: word.String(), dynamic: !quoted && p.wordDynamic(word.String()), quoted: quoted})
				word.Reset()
				wordStarted = false
				quoted = false
			}
			operator := string(r)
			if index+1 < len(runes) && (runes[index+1] == r || runes[index+1] == '&') {
				index++
				operator += string(runes[index])
			}
			result = append(result, shellLexeme{text: operator, operator: true, redirect: true, output: r == '>'})
		default:
			wordStarted = true
			word.WriteRune(r)
		}
	}
	if wordStarted {
		result = append(result, shellLexeme{text: word.String(), dynamic: !quoted && p.wordDynamic(word.String()), quoted: quoted})
	}
	return result, true
}

func (p *shellParser) wordDynamic(word string) bool {
	return strings.ContainsAny(word, "*?[]") || (strings.Contains(word, "{") && strings.Contains(word, ",")) || strings.Contains(word, "$")
}

func shellFileDescriptor(value string) bool {
	if value == "-" || value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func shellInsecureChmod(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		lower := strings.ToLower(arg)
		if lower == "777" || lower == "666" || lower == "0777" || lower == "0666" || strings.ContainsAny(lower, "w") && (strings.Contains(lower, "a+") || strings.Contains(lower, "o+") || strings.Contains(lower, "g+") || strings.Contains(lower, "s")) {
			return true
		}
	}
	return false
}

func shellSubstitutionEnd(runes []rune, start int) int {
	depth := 0
	for index := start; index < len(runes); index++ {
		switch runes[index] {
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

func shellCommandStart(words []string) int {
	index := 0
	for index < len(words) {
		if strings.Contains(words[index], "=") && !strings.HasPrefix(words[index], "=") {
			index++
			continue
		}
		if words[index] == "env" || words[index] == "command" || words[index] == "exec" {
			index++
			continue
		}
		return index
	}
	return len(words)
}

func shellExecutable(word string) string {
	word = strings.ToLower(strings.TrimSpace(strings.Trim(word, "'\"")))
	if slash := strings.LastIndexAny(word, "/\\"); slash >= 0 {
		word = word[slash+1:]
	}
	return strings.TrimSuffix(word, ".exe")
}

func shellHelpOnly(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "--version" || args[0] == "-h")
}

func knownShellCommand(command string) bool {
	switch command {
	case "cat", "cut", "date", "du", "echo", "head", "less", "ls", "more", "printf", "pwd", "sort", "stat", "tail", "type", "uname", "wc", "which", "true", "false", "test", "grep", "rg", "find", "git", "docker", "kubectl", "oc", "curl", "wget":
		return true
	default:
		return false
	}
}

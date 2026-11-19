package security

const (
	safetyEraseCommand        = "erase"
	safetyDoasCommand         = "doas"
	safetyChmodCommand        = "chmod"
	safetyFormatCommand       = "format"
	safetyCatCommand          = "cat"
	safetyCommandCommand      = "command"
	safetyBashCommand         = "bash"
	safetyRemoveItemCommand   = "remove-item"
	safetySudoCommand         = "sudo"
	safetyFormatVolumeCommand = "format-volume"
	safetyHeadCommand         = "head"
	safetyEnvCommand          = "env"
	safetyCmdCommand          = "cmd"
	safetyDashCommand         = "dash"
	safetyRmdirCommand        = "rmdir"
	safetyMkfsCommand         = "mkfs"
	safetyLessCommand         = "less"
	safetyExecCommand         = "exec"
	safetyFishCommand         = "fish"
	safetyKshCommand          = "ksh"
	safetyNewfsCommand        = "newfs"
	safetyMoreCommand         = "more"
	safetyPowerShellCommand   = "powershell"
	safetyZshCommand          = "zsh"
	safetyTailCommand         = "tail"
	safetyPwshCommand         = "pwsh"
	safetyTypeCommand         = "type"
)

// DefaultDangerousCommands lists commands whose common destructive forms are
// treated as dangerous by the local command assessment.
var DefaultDangerousCommands = []string{
	"rm", safetyRmdirCommand, "unlink", "del", safetyEraseCommand, "rd", safetyRemoveItemCommand,
	"dd", safetyMkfsCommand, "mkfs.*", safetyNewfsCommand, safetyFormatCommand, safetyFormatVolumeCommand,
	safetySudoCommand, safetyDoasCommand, safetyChmodCommand, "chown", "setfacl", "eval", "source", ".",
	"sh", safetyBashCommand, safetyDashCommand, safetyZshCommand, safetyKshCommand, safetyFishCommand, safetyPwshCommand, safetyPowerShellCommand, safetyCmdCommand,
	"docker prune", "docker rm", "docker rmi", "docker kill", "docker stop", "docker compose down",
	"kubectl delete", "kubectl drain", "kubectl cordon", "kubectl uncordon",
	"oc delete", "oc drain", "oc cordon", "oc uncordon",
	"find -delete", "find -exec", "find -execdir",
	"git reset --hard", "git clean -f", "git push --force",
}

// DefaultSensitivePathMarkers identifies path fragments that make a read
// ineligible for automatic classification as known-safe.
var DefaultSensitivePathMarkers = []string{
	".env", "/.ssh/", "/.aws/", "/.zev/", "credential", "secret", "password",
	"token", "private_key", "private-key", "id_rsa", "id_ed25519",
}

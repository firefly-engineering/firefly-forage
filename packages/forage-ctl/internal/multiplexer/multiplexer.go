// Package multiplexer provides an abstraction over terminal multiplexers
// (tmux, wezterm) used for sandbox sessions.
package multiplexer

// Type identifies a terminal multiplexer backend.
type Type string

const (
	TypeTmux    Type = "tmux"
	TypeWezterm Type = "wezterm"
)

// Window describes a multiplexer window/tab to create at sandbox start.
type Window struct {
	Name    string
	Command string
}

// ConfigMount describes a host file or directory to bind-mount into the
// container so the multiplexer picks up the user's configuration.
type ConfigMount struct {
	ContainerPath string
	HostPath      string
	ReadOnly      bool
}

// Multiplexer is the interface that every multiplexer backend implements.
type Multiplexer interface {
	// Type returns the multiplexer type identifier.
	Type() Type

	// NixPackages returns the Nix package names to install in the container.
	NixPackages() []string

	// InitScript returns the shell script body for the forage-init service
	// that creates a session and spawns the given windows.
	InitScript(windows []Window) string

	// AttachCommand returns the SSH remote command used to attach to the
	// session. An empty string means native attach (no SSH command needed).
	AttachCommand() string

	// CheckSessionArgs returns the SSH command+args used to test whether a
	// session is running inside the container.
	CheckSessionArgs() []string

	// ListWindowsArgs returns the SSH command+args used to list windows.
	ListWindowsArgs() []string

	// ParseWindowList parses the output of the list-windows command into
	// a slice of human-readable window descriptions.
	ParseWindowList(output string) []string

	// HostConfigMounts returns bind mounts for the user's multiplexer
	// configuration on the host.
	HostConfigMounts(homeDir string) []ConfigMount

	// PromptInstructions returns a short string for agent system prompts
	// describing how to use the multiplexer.
	PromptInstructions() string
}

// New returns a Multiplexer for the given type.
// Defaults to TypeTmux for empty or unrecognised values.
func New(t Type) Multiplexer {
	switch t {
	case TypeWezterm:
		return &Wezterm{}
	default:
		return &Tmux{}
	}
}

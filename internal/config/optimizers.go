package config

// Optimizer is a program that rewrites shell commands so their output costs
// fewer tokens (HRN-019). It runs after the broker has approved the original
// command and has no say in what may run.
type Optimizer struct {
	ID   string
	Name string
	// Program is looked up the way the session's commands find it. A rewrite
	// may only put this word in front of a command.
	Program string
	// Rewrite are the arguments before the command to rewrite. A run that
	// exits with one of RewriteExits printed the new command; any other exit
	// leaves the command as it is.
	Rewrite      []string
	RewriteExits []int
	// RewriteEnv is added for the rewrite itself.
	RewriteEnv map[string]string
	// ProbeCommand must rewrite to ProbeWant, which tells this program from
	// another with the same name.
	ProbeCommand, ProbeWant string
	// Env is added to every command while the optimizer is on, and DataEnv
	// points variables at files in the session's own folder.
	Env     map[string]string
	DataEnv map[string]string
	// Install tells the user how to get the program.
	Install string
}

// BuiltinOptimizers are the optimizers every install offers.
func BuiltinOptimizers() []Optimizer {
	return []Optimizer{{
		ID: "rtk", Name: "RTK", Program: "rtk",
		Rewrite: []string{"rewrite"}, RewriteExits: []int{0, 3},
		// Without a home it reads no agent's settings: what may run is the
		// broker's business.
		RewriteEnv:   map[string]string{"HOME": "/nonexistent"},
		ProbeCommand: "git status", ProbeWant: "rtk git status",
		Env: map[string]string{
			"RTK_TELEMETRY_DISABLED":    "1",
			"RTK_ONRAMP_DISABLED":       "1",
			"RTK_SUPPRESS_HOOK_WARNING": "1",
		},
		// Full outputs are kept here, for `rtk recall`, so they go when the
		// session goes.
		DataEnv: map[string]string{"RTK_DB_PATH": "tracking.db", "RTK_RECALL_DB": "recall.db"},
		Install: "brew install rtk",
	}}
}

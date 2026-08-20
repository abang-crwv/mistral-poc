// Command qac is the QAgenticCow firmware-release canary verification tool.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	// Load creds from .env (cwd) before any command reads the environment.
	// Real environment variables take precedence — see loadDotEnv. .env is
	// gitignored; never commit secrets. Log only variable NAMES (not values)
	// so a misformatted .env is diagnosable from the startup output.
	if loaded, present, malformed := loadDotEnv(".env"); present {
		if len(loaded) > 0 {
			fmt.Fprintf(os.Stderr, "qac: loaded %d var(s) from .env: %v\n", len(loaded), loaded)
		}
		if malformed > 0 {
			fmt.Fprintf(os.Stderr, "qac: .env present but %d non-empty line(s) had no KEY=VALUE form (skipped)\n", malformed)
		}
		if len(loaded) == 0 && malformed == 0 {
			fmt.Fprintln(os.Stderr, "qac: .env present but empty (only blanks/comments)")
		}
	}
	// Accept common bare credential names (SOURCEGRAPH_TOKEN, VM_USER, …) as
	// aliases for qac's canonical AWXCTL_* variables.
	if filled := applyEnvAliases(); len(filled) > 0 {
		fmt.Fprintf(os.Stderr, "qac: resolved creds via aliases: %v\n", filled)
	}

	root := &cobra.Command{
		Use:   "qac",
		Short: "QAgenticCow — firmware-release canary verification",
	}
	root.AddCommand(serveCmd())
	root.AddCommand(seedDemoCmd())
	root.AddCommand(templateCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

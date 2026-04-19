package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommandMeta is the structured-JSON shape returned by `--help --agent`.
// Agents walk this tree to discover the CLI surface — the per-command drill-down
// that complements the flat catalog from `kestrel commands --json`.
type CommandMeta struct {
	Command        string       `json:"command"`
	Path           string       `json:"path"`
	Short          string       `json:"short,omitempty"`
	Long           string       `json:"long,omitempty"`
	Usage          string       `json:"usage,omitempty"`
	Aliases        []string     `json:"aliases,omitempty"`
	Subcommands    []SubCommand `json:"subcommands,omitempty"`
	Flags          []FlagMeta   `json:"flags,omitempty"`
	InheritedFlags []FlagMeta   `json:"inherited_flags,omitempty"`
}

type SubCommand struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Short string `json:"short,omitempty"`
}

type FlagMeta struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage,omitempty"`
}

// agentHelpFunc is set on every command. When `--agent` (or `--json`) is
// present on the command line, it emits structured metadata instead of the
// usual styled help output.
//
// We sniff os.Args because cobra's flag parsing for help is idiosyncratic —
// checking flag state can miss bindings in some subcommand contexts.
func agentHelpFunc(cmd *cobra.Command, _ []string) {
	if !agentHelpRequested() {
		defaultHelp(cmd)
		return
	}
	meta := buildCommandMeta(cmd)
	out, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		defaultHelp(cmd)
		return
	}
	fmt.Println(string(out))
}

func agentHelpRequested() bool {
	for _, a := range os.Args {
		if a == "--agent" || a == "--json" {
			return true
		}
	}
	return false
}

func defaultHelp(cmd *cobra.Command) {
	// Restore cobra's default help rendering.
	cmd.Root().SetHelpFunc(nil)
	_ = cmd.Help()
	cmd.Root().SetHelpFunc(agentHelpFunc)
}

func buildCommandMeta(cmd *cobra.Command) CommandMeta {
	meta := CommandMeta{
		Command: cmd.Name(),
		Path:    cmd.CommandPath(),
		Short:   cmd.Short,
		Long:    strings.TrimSpace(cmd.Long),
		Usage:   cmd.UseLine(),
		Aliases: cmd.Aliases,
	}

	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		meta.Subcommands = append(meta.Subcommands, SubCommand{
			Name:  sub.Name(),
			Path:  sub.CommandPath(),
			Short: sub.Short,
		})
	}

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		meta.Flags = append(meta.Flags, flagMetaFrom(f))
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		meta.InheritedFlags = append(meta.InheritedFlags, flagMetaFrom(f))
	})
	return meta
}

func flagMetaFrom(f *pflag.Flag) FlagMeta {
	return FlagMeta{
		Name:      f.Name,
		Shorthand: f.Shorthand,
		Type:      f.Value.Type(),
		Default:   f.DefValue,
		Usage:     f.Usage,
	}
}

// wireAgentHelp attaches the help function to the root so every subcommand
// inherits it. Called once from init.
func wireAgentHelp() {
	rootCmd.SetHelpFunc(agentHelpFunc)
}

func init() {
	wireAgentHelp()
}

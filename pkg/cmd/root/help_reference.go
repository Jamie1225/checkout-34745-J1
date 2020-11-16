package root

import (
	"fmt"
	"strings"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/spf13/cobra"
)

func referenceHelpFn(cmd *cobra.Command, io *iostreams.IOStreams) func() string {
	cs := io.ColorScheme()
	return func() string {
		reftext := "# gh reference\n\n"

		for _, c := range cmd.Commands() {
			reftext += cmdRef(cs, c, 0)
		}

		style := markdown.GetStyle(io.DetectTerminalTheme())
		md, err := markdown.RenderWrap(reftext, style, io.TerminalWidth())
		if err != nil {
			return reftext
		}

		return md
	}
}

func cmdRef(cs *iostreams.ColorScheme, cmd *cobra.Command, lvl int) string {
	ref := ""

	if cmd.Hidden {
		return ref
	}

	cmdPrefix := "##"
	if lvl > 0 {
		cmdPrefix = "###"
	}

	// Name + Description
	escaped := strings.ReplaceAll(cmd.Use, "<", "&lt;")
	escaped = strings.ReplaceAll(escaped, ">", "&gt;")
	ref += fmt.Sprintf("%s %s\n\n", cmdPrefix, escaped)

	ref += cmd.Short + "\n\n"

	// Flags

	// TODO glamour doesn't respect linebreaks (double space or backslash at end) at all, so there is
	// no way to have the damn flags print without a whole newline in between.  I tried generating my
	// own usage (to eexperiment with other ways of rendering in markdown) but there isn't enough
	// exposed in pflag to produce the same quality of output as their FlagUsages method.
	if cmd.HasPersistentFlags() || cmd.HasFlags() {
		for _, fu := range strings.Split(cmd.Flags().FlagUsages(), "\n") {
			if fu == "" {
				continue
			}
			ref += fu + "\n\n"
		}
		for _, fu := range strings.Split(cmd.PersistentFlags().FlagUsages(), "\n") {
			if fu == "" {
				continue
			}
			ref += fu + "\n\n"
		}

		ref += "\n"
	} else if cmd.HasAvailableSubCommands() {
		ref += "\n"

	}

	// Subcommands
	subcommands := cmd.Commands()
	for _, c := range subcommands {
		ref += cmdRef(cs, c, lvl+1)
	}

	if len(subcommands) == 0 {
		ref += "\n"
	}

	return ref
}

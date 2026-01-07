// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package figspec

import (
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
)

// ExtensionSpecLoader loads command specifications for extensions.
type ExtensionSpecLoader interface {
	// LoadExtensionSpec loads the command spec for an extension by ID.
	// Returns nil if the extension is not found or has no command spec.
	LoadExtensionSpec(extensionID string) *extensions.CommandSpec
}

// extensionSpecLoaderFunc is a function adapter for ExtensionSpecLoader
type extensionSpecLoaderFunc func(extensionID string) *extensions.CommandSpec

func (f extensionSpecLoaderFunc) LoadExtensionSpec(extensionID string) *extensions.CommandSpec {
	return f(extensionID)
}

// NewExtensionSpecLoader creates an ExtensionSpecLoader from a function.
func NewExtensionSpecLoader(fn func(extensionID string) *extensions.CommandSpec) ExtensionSpecLoader {
	return extensionSpecLoaderFunc(fn)
}

// convertCommandSpecToSubcommands converts extension CommandEntry to Fig Subcommands.
func convertCommandSpecToSubcommands(commands []extensions.CommandEntry) []Subcommand {
	var subcommands []Subcommand

	for _, cmd := range commands {
		subcommand := Subcommand{
			Name:        []string{cmd.Name},
			Description: cmd.Description,
			Subcommands: convertCommandSpecToSubcommands(cmd.Subcommands),
			Options:     convertFlagsToOptions(cmd.Flags),
		}
		subcommands = append(subcommands, subcommand)
	}

	return subcommands
}

// convertFlagsToOptions converts extension FlagEntry to Fig Options.
func convertFlagsToOptions(flags []extensions.FlagEntry) []Option {
	var options []Option

	for _, flag := range flags {
		names := []string{"--" + flag.Name}
		if flag.Shorthand != "" {
			names = append(names, "-"+flag.Shorthand)
		}

		option := Option{
			Name:        names,
			Description: flag.Description,
		}

		// Add argument if flag takes a value
		if flag.TakesValue {
			option.Args = []Arg{{Name: flag.Name}}
		}

		options = append(options, option)
	}

	return options
}

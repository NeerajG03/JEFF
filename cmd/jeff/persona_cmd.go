package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF/persona"
	"github.com/spf13/cobra"
)

func personaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "persona",
		Short: "Manage agent personas",
	}
	cmd.AddCommand(
		personaListCmd(),
		personaShowCmd(),
		personaAddCmd(),
		personaRemoveCmd(),
		personaTagCmd(),
	)
	return cmd
}

func personaListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered personas",
		RunE: func(cmd *cobra.Command, args []string) error {
			personas, err := persona.ListPersonas(cfg.Home)
			if err != nil {
				return err
			}
			if len(personas) == 0 {
				fmt.Println("No personas registered. Run `jeff init --update` to seed defaults.")
				return nil
			}

			maxName := 4
			for _, p := range personas {
				if len(p.Name) > maxName {
					maxName = len(p.Name)
				}
			}

			fmt.Printf("  %-*s  %-35s  %s\n", maxName, "NAME", "DESCRIPTION", "MEMORY HINT")
			for _, p := range personas {
				desc := p.Entry.Description
				if desc == "" {
					desc = "—"
				}
				hint := p.Entry.MemoryHint
				if hint == "" {
					hint = "—"
				}
				if len(hint) > 50 {
					hint = hint[:47] + "..."
				}
				fmt.Printf("  %-*s  %-35s  %s\n", maxName, p.Name, desc, hint)
			}
			return nil
		},
	}
}

func personaShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show <name>",
		Short:             "Show persona details",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: registeredPersonaCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := persona.GetPersona(cfg.Home, args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name:        %s\n", args[0])
			fmt.Printf("Location:    %s\n", entry.Location)
			if entry.Description != "" {
				fmt.Printf("Description: %s\n", entry.Description)
			}
			if entry.MemoryHint != "" {
				fmt.Printf("Memory hint: %s\n", entry.MemoryHint)
			}

			// Show PERSONA.md preview.
			personaMD := filepath.Join(entry.Location, "PERSONA.md")
			data, err := os.ReadFile(personaMD)
			if err != nil {
				fmt.Printf("\n(PERSONA.md not readable: %v)\n", err)
				return nil
			}

			fmt.Printf("\n--- PERSONA.md ---\n")
			lines := strings.Split(string(data), "\n")
			limit := 30
			if len(lines) < limit {
				limit = len(lines)
			}
			for _, line := range lines[:limit] {
				fmt.Println(line)
			}
			if len(lines) > 30 {
				fmt.Printf("... (%d more lines)\n", len(lines)-30)
			}
			return nil
		},
	}
}

func personaAddCmd() *cobra.Command {
	var external bool
	var name string

	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Register a persona from a directory containing PERSONA.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := persona.AddPersona(cfg.Home, args[0], name, external)
			if err != nil {
				return err
			}
			if name == "" {
				name = filepath.Base(args[0])
			}
			fmt.Printf("Added persona %s → %s\n", name, entry.Location)
			return nil
		},
	}

	cmd.Flags().BoolVar(&external, "external", false, "Register without copying (persona stays at original location)")
	cmd.Flags().StringVar(&name, "name", "", "Persona name (defaults to directory name)")
	return cmd
}

func personaRemoveCmd() *cobra.Command {
	var deleteFiles bool

	cmd := &cobra.Command{
		Use:               "remove <name>",
		Short:             "Unregister a persona",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: registeredPersonaCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := persona.RemovePersona(cfg.Home, args[0], deleteFiles); err != nil {
				return err
			}
			fmt.Printf("Removed persona %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteFiles, "delete", false, "Also delete persona files (only if stored in .personas/)")
	return cmd
}

func personaTagCmd() *cobra.Command {
	var description, memoryHint, model string

	cmd := &cobra.Command{
		Use:               "tag <name>",
		Short:             "Update persona description, memory hint, and model",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: registeredPersonaCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			d := ""
			m := ""
			mdl := ""
			if cmd.Flags().Changed("description") {
				d = description
			}
			if cmd.Flags().Changed("memory-hint") {
				m = memoryHint
			}
			if cmd.Flags().Changed("model") {
				mdl = model
			}
			if d == "" && m == "" && mdl == "" {
				return fmt.Errorf("specify --description, --memory-hint, and/or --model")
			}
			if err := persona.UpdatePersona(cfg.Home, args[0], d, m, mdl); err != nil {
				return err
			}
			fmt.Printf("Updated persona %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Short role description")
	cmd.Flags().StringVar(&memoryHint, "memory-hint", "", "What this persona should capture in the scratchpad")
	cmd.Flags().StringVar(&model, "model", "", "Default Claude model (e.g. sonnet, opus, haiku)")
	return cmd
}

// registeredPersonaCompletion completes registered persona names.
func registeredPersonaCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return persona.RegisteredNamesWithDescriptions(cfg.Home), cobra.ShellCompDirectiveNoFileComp
}

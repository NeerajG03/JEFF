package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NeerajG03/JEFF/skill"
	"github.com/spf13/cobra"
)

func skillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage agent skills",
	}
	cmd.AddCommand(
		skillDocCmd(),
		skillListCmd(),
		skillShowCmd(),
		skillAddCmd(),
		skillRemoveCmd(),
		skillTagCmd(),
		skillInjectCmd(),
		skillEjectCmd(),
	)
	return cmd
}

func skillDocCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doc",
		Short: "Print the skill management guide",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(skill.Doc)
		},
	}
}

func skillListCmd() *cobra.Command {
	var persona, gigType, tag string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := skill.List(cfg.Home)
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				fmt.Println("No skills registered.")
				return nil
			}

			// Optional filtering.
			if persona != "" || gigType != "" || tag != "" {
				ctx := &skill.MatchContext{Persona: persona, GigType: gigType}
				if tag != "" {
					ctx.Labels = []string{tag}
				}
				var filtered []*skill.SkillInfo
				for _, s := range skills {
					if skill.Match(s.Entry, ctx) {
						filtered = append(filtered, s)
					}
				}
				skills = filtered
				if len(skills) == 0 {
					fmt.Println("No matching skills.")
					return nil
				}
			}

			// Find max name width.
			maxName := 4
			for _, s := range skills {
				if len(s.Name) > maxName {
					maxName = len(s.Name)
				}
			}

			fmt.Printf("  %-*s  %-12s  %-16s  %s\n", maxName, "NAME", "PERSONAS", "TYPES", "TAGS")
			for _, s := range skills {
				personas := "(all)"
				if len(s.Entry.Personas) > 0 {
					personas = strings.Join(s.Entry.Personas, ", ")
				}
				types := "(all)"
				if len(s.Entry.GigTypes) > 0 {
					types = strings.Join(s.Entry.GigTypes, ", ")
				}
				tags := ""
				if len(s.Entry.Tags) > 0 {
					tags = strings.Join(s.Entry.Tags, ", ")
				}
				fmt.Printf("  %-*s  %-12s  %-16s  %s\n", maxName, s.Name, personas, types, tags)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&persona, "persona", "", "Filter by persona")
	cmd.Flags().StringVar(&gigType, "type", "", "Filter by gig task type")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	return cmd
}

func skillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show skill details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := skill.Get(cfg.Home, args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name:      %s\n", args[0])
			fmt.Printf("Location:  %s\n", entry.Location)
			if len(entry.Personas) > 0 {
				fmt.Printf("Personas:  %s\n", strings.Join(entry.Personas, ", "))
			}
			if len(entry.GigTypes) > 0 {
				fmt.Printf("Types:     %s\n", strings.Join(entry.GigTypes, ", "))
			}
			if len(entry.Tags) > 0 {
				fmt.Printf("Tags:      %s\n", strings.Join(entry.Tags, ", "))
			}

			// Show SKILL.md preview.
			skillMD := filepath.Join(entry.Location, "SKILL.md")
			data, err := os.ReadFile(skillMD)
			if err != nil {
				fmt.Printf("\n(SKILL.md not readable: %v)\n", err)
				return nil
			}

			fmt.Printf("\n--- SKILL.md ---\n")
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

func skillAddCmd() *cobra.Command {
	var external bool
	var name string

	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Register a skill from a directory containing SKILL.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := skill.Add(cfg.Home, args[0], name, external)
			if err != nil {
				return err
			}
			if name == "" {
				name = filepath.Base(args[0])
			}
			fmt.Printf("Added skill %s → %s\n", name, entry.Location)
			return nil
		},
	}

	cmd.Flags().BoolVar(&external, "external", false, "Register without copying (skill stays at original location)")
	cmd.Flags().StringVar(&name, "name", "", "Skill name (defaults to directory name)")
	return cmd
}

func skillRemoveCmd() *cobra.Command {
	var deleteFiles bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := skill.Remove(cfg.Home, args[0], deleteFiles); err != nil {
				return err
			}
			fmt.Printf("Removed skill %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteFiles, "delete", false, "Also delete skill files (only if stored in .skills/)")
	return cmd
}

func skillTagCmd() *cobra.Command {
	var personas, types, tags []string

	cmd := &cobra.Command{
		Use:   "tag <name>",
		Short: "Set injection tags for a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var p, g, t []string
			if cmd.Flags().Changed("persona") {
				p = personas
			}
			if cmd.Flags().Changed("type") {
				g = types
			}
			if cmd.Flags().Changed("tag") {
				t = tags
			}
			if err := skill.SetTags(cfg.Home, args[0], p, g, t); err != nil {
				return err
			}
			fmt.Printf("Updated tags for %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&personas, "persona", nil, "Persona names (captain, nerd, jock, scout)")
	cmd.Flags().StringSliceVar(&types, "type", nil, "Gig task types (task, bug, feature, epic, chore)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Free-form tags matched against task labels")
	return cmd
}

func skillInjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inject <name>",
		Short: "Inject a skill into the current task workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, taskDir, err := resolveTaskID(nil)
			if err != nil {
				return err
			}

			entry, err := skill.Get(cfg.Home, args[0])
			if err != nil {
				return err
			}

			if err := skill.Inject(args[0], entry.Location, taskDir); err != nil {
				return err
			}
			fmt.Printf("Injected %s into %s\n", args[0], filepath.Base(taskDir))
			return nil
		},
	}
}

func skillEjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "eject <name>",
		Short: "Remove a skill from the current task workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, taskDir, err := resolveTaskID(nil)
			if err != nil {
				return err
			}

			if err := skill.Eject(args[0], taskDir); err != nil {
				return err
			}
			fmt.Printf("Ejected %s from %s\n", args[0], filepath.Base(taskDir))
			return nil
		},
	}
}

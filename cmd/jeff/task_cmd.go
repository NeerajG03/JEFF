package main

import (
	"encoding/json"
	"fmt"

	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage gig tasks (thin wrapper over gig SDK)",
	}
	cmd.AddCommand(taskNewCmd())
	cmd.AddCommand(taskListCmd())
	cmd.AddCommand(taskShowCmd())
	return cmd
}

func taskNewCmd() *cobra.Command {
	var taskType, parentID string
	var priority int
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a new task (prints ID on its own line)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isGigStoreInitialized(cfg) {
				return fmt.Errorf("gig store not initialized — run `gig init --prefix <name>` first")
			}

			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			task, err := store.Create(gig.CreateParams{
				Title:    args[0],
				Type:     gig.TaskType(taskType),
				Priority: gig.Priority(priority),
				ParentID: parentID,
			})
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				return enc.Encode(task)
			}
			fmt.Fprintln(cmd.OutOrStdout(), task.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&taskType, "type", "task", "Task type (task|bug|feature|epic|chore)")
	cmd.Flags().IntVar(&priority, "priority", 2, "Priority (0=critical, 4=backlog)")
	cmd.Flags().StringVar(&parentID, "parent", "", "Parent task ID")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskListCmd() *cobra.Command {
	var ready, mine bool
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			var tasks []*gig.Task
			if ready {
				tasks, err = store.Ready("")
			} else if mine {
				user := "" // TODO: detect current user
				tasks, err = store.List(gig.ListParams{Assignee: user})
			} else {
				tasks, err = store.List(gig.ListParams{})
			}
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				return enc.Encode(tasks)
			}

			if len(tasks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tasks found.")
				return nil
			}

			for _, t := range tasks {
				assignee := ""
				if t.Assignee != "" {
					assignee = " @" + t.Assignee
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s %s %s%s\n", t.ID, colorStatus(t.Status), colorPriority(t.Priority), t.Title, assignee)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ready, "ready", false, "Show ready tasks (open, no blockers)")
	cmd.Flags().BoolVar(&mine, "mine", false, "Show tasks assigned to you")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")

	return cmd
}

func taskShowCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openGigStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			task, err := store.GetFull(args[0])
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				return enc.Encode(task)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s  %s\n", colorize(cDim, "ID:"), task.ID)
			fmt.Fprintf(out, "%s   %s\n", colorize(cDim, "Title:"), task.Title)
			fmt.Fprintf(out, "%s  %s %s\n", colorize(cDim, "Status:"), colorStatus(task.Status), string(task.Status))
			fmt.Fprintf(out, "%s %s %s\n", colorize(cDim, "Priority:"), colorPriority(task.Priority), colorize(cDim, task.Priority.String()))
			fmt.Fprintf(out, "%s    %s\n", colorize(cDim, "Type:"), string(task.Type))
			if task.Assignee != "" {
				fmt.Fprintf(out, "%s %s\n", colorize(cDim, "Assignee:"), task.Assignee)
			}
			if task.ParentID != "" {
				fmt.Fprintf(out, "%s  %s\n", colorize(cDim, "Parent:"), task.ParentID)
			}
			if task.Description != "" {
				fmt.Fprintf(out, "%s    %s\n", colorize(cDim, "Desc:"), task.Description)
			}
			fmt.Fprintf(out, "%s %s\n", colorize(cDim, "Created:"), task.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(out, "%s %s\n", colorize(cDim, "Updated:"), task.UpdatedAt.Format("2006-01-02 15:04:05"))
			if task.ClosedAt != nil {
				fmt.Fprintf(out, "%s  %s\n", colorize(cDim, "Closed:"), task.ClosedAt.Format("2006-01-02 15:04:05"))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

package cli

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type rcaRevisionRow struct {
	ID               string  `json:"id"`
	Revision         int     `json:"revision"`
	Title            string  `json:"title"`
	Summary          *string `json:"summary"`
	ObservationState string  `json:"observation_state"`
	CreatedBy        struct {
		Label string `json:"label"`
	} `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

type rcaBlockRow struct {
	Kind       string `json:"kind"`
	Markdown   string `json:"markdown"`
	SnapshotID string `json:"snapshot_id"`
}

type rcaListBody struct {
	RCAs []rcaRevisionRow `json:"rcas"`
}

type rcaShownBody struct {
	RCA struct {
		rcaRevisionRow
		Blocks []rcaBlockRow `json:"blocks"`
	} `json:"rca"`
}

func (a *app) observationEditCommand() *cobra.Command {
	var title, from, to string
	var clearRange bool
	cmd := &cobra.Command{
		Use:   "edit <observation-id> [--title <title>] [--from <time> --to <time> | --clear-range]",
		Short: "Rename an observation or change the range its panels show",
		Long:  "edit changes the title, the range or both.\n--from and --to, RFC 3339 times given together, set the range its panels show, and --clear-range removes it.",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			body, err := editBody(title, from, to, clearRange)
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPatch, "observations/"+id, body, func(out *output, data []byte) error {
				edited, err := decode[observationEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Updated %s", text(edited.Observation.Title))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "the new title")
	cmd.Flags().StringVar(&from, "from", "", "start of the range its panels show, an RFC 3339 time")
	cmd.Flags().StringVar(&to, "to", "", "end of the range its panels show, an RFC 3339 time")
	cmd.Flags().BoolVar(&clearRange, "clear-range", false, "remove the range its panels show")
	return cmd
}

func editBody(title, from, to string, clearRange bool) (map[string]any, error) {
	if (from == "") != (to == "") {
		return nil, usagef("--from and --to are given together")
	}
	if clearRange && from != "" {
		return nil, usagef("--clear-range cannot be used with --from and --to")
	}
	if title == "" && from == "" && !clearRange {
		return nil, usagef("give --title, --from with --to, or --clear-range")
	}
	body := map[string]any{}
	if title != "" {
		body["title"] = title
	}
	switch {
	case clearRange:
		body["from"], body["to"] = nil, nil
	case from != "":
		body["from"], body["to"] = from, to
	}
	return body, nil
}

func (a *app) observationMoveCommand() *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "move <observation-id> --folder <folder-id>",
		Short: "Move an observation to another folder",
		Long:  "move files an observation in another folder.\nYou need Edit on the observation and on the target folder, and a personal folder (My Dashboards) takes no moves.",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if folder == "" {
				return usagef("--folder <folder-id> is required")
			}
			target, err := parseID(folder)
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPost, "observations/"+id+"/move", map[string]string{"folder_id": target}, func(out *output, data []byte) error {
				moved, err := decode[observationEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Moved %s to folder %s", text(moved.Observation.Title), text(target))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "the folder id to move it to")
	return cmd
}

func (a *app) observationReopenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reopen <observation-id>",
		Short: "Reopen a resolved observation",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPost, "observations/"+id+"/reopen", nil, func(out *output, data []byte) error {
				reopened, err := decode[observationEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Reopened %s", text(reopened.Observation.Title))
				return nil
			})
		},
	}
}

func (a *app) observationRCAsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rcas <observation-id> [<revision>]",
		Short: "List the published RCA revisions of an observation, or read one",
		Long:  "rcas lists the revisions published from an observation, newest first.\nWith a revision number it prints that revision: its title, summary and text, and one line for each snapshot.",
		Args:  rangeArgs(1, 2, "one observation id and an optional revision number"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return a.observationCall(cmd, http.MethodGet, "observations/"+id+"/rcas", nil, printRCAList)
			}
			revision, err := strconv.Atoi(args[1])
			if err != nil || revision < 1 {
				return usagef("%q is not a revision; a revision is a positive number", text(args[1]))
			}
			return a.observationCall(cmd, http.MethodGet, "observations/"+id+"/rcas/"+strconv.Itoa(revision), nil, printRCA)
		},
	}
}

func printRCAList(out *output, data []byte) error {
	listed, err := decode[rcaListBody](data)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(listed.RCAs))
	for _, r := range listed.RCAs {
		rows = append(rows, []string{strconv.Itoa(r.Revision), text(r.Title), text(r.ObservationState), text(r.CreatedBy.Label), when(r.CreatedAt)})
	}
	out.table([]string{"REVISION", "TITLE", "STATE", "PUBLISHED BY", "PUBLISHED"}, rows)
	return nil
}

func printRCA(out *output, data []byte) error {
	shown, err := decode[rcaShownBody](data)
	if err != nil {
		return err
	}
	rca := shown.RCA
	out.line("%s (revision %d)", text(rca.Title), rca.Revision)
	if rca.Summary != nil && *rca.Summary != "" {
		out.line("%s", text(*rca.Summary))
	}
	for _, block := range rca.Blocks {
		out.line("")
		switch block.Kind {
		case "text":
			for line := range strings.SplitSeq(block.Markdown, "\n") {
				out.line("%s", text(line))
			}
		case "snapshot":
			out.line("snapshot %s", text(block.SnapshotID))
		}
	}
	return nil
}

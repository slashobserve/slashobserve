package cli

import (
	"net/http"

	"github.com/spf13/cobra"
)

type observationPin struct {
	DashboardID  string `json:"dashboard_id"`
	AnnotationID string `json:"annotation_id"`
}

func (a *app) observationPinCommand() *cobra.Command {
	var dashboard string
	cmd := &cobra.Command{
		Use:   "pin <observation-id> [--dashboard <dashboard-id>]",
		Short: "Pin an observation to a dashboard as an annotation",
		Long:  "pin adds an annotation with the observation's title and a link to it, at the end of its range, and needs edit access to the dashboard.\n--dashboard is by default the dashboard the observation's run was about; an observation with none answers observation_has_no_dashboard.\nPinning it again to the same dashboard changes nothing.",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			body := map[string]string{}
			if dashboard != "" {
				target, err := parseID(dashboard)
				if err != nil {
					return err
				}
				body["dashboard_id"] = target
			}
			return a.observationCall(cmd, http.MethodPost, "observations/"+id+"/pin", body, func(out *output, data []byte) error {
				pin, err := decode[observationPin](data)
				if err != nil {
					return err
				}
				out.confirm("Pinned to dashboard %s as annotation %s", text(pin.DashboardID), text(pin.AnnotationID))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&dashboard, "dashboard", "", "the dashboard id to pin to")
	return cmd
}

func (a *app) observationDashboardCommand() *cobra.Command {
	var block string
	var again bool
	cmd := &cobra.Command{
		Use:   "dashboard <observation-id> [--block <block-id>] [--new]",
		Short: "Make a dashboard from the panel blocks of an observation",
		Long:  "dashboard saves the live panel blocks, or one block with --block, as a new dashboard in My Dashboards, with the variables each block carries; every query is run again first.\nMaking it again returns the dashboard you made first; --new makes another.\nAn observation without a live panel block answers observation_has_no_panels.",
		Args:  exactArgs(1, "one observation id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			path := "observations/" + id + "/dashboard"
			if block != "" {
				blockID, err := parseID(block)
				if err != nil {
					return err
				}
				path = "blocks/" + blockID + "/dashboard"
			}
			return a.observationCall(cmd, http.MethodPost, path, map[string]bool{"new": again}, func(out *output, data []byte) error {
				made, err := decode[dashboardWithVersion](data)
				if err != nil {
					return err
				}
				out.confirm("Dashboard %s: %s", text(made.Dashboard.ID), text(made.Dashboard.Title))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&block, "block", "", "make it from this panel block only")
	cmd.Flags().BoolVar(&again, "new", false, "make another dashboard even when you already made one")
	return cmd
}

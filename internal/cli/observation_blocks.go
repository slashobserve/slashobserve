package cli

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type blockEditOptions struct {
	source     textSource
	panelFile  string
	from, to   string
	version    int
	half       bool
	full       bool
	width      string
	height     int
	position   int
	followPage bool
}

func (o blockEditOptions) body(cmd *cobra.Command) (map[string]any, error) {
	if o.version < 1 {
		return nil, usagef("--version <n> is required, the block's version from observe o show")
	}
	if err := checkWidthFlags(o.width, o.half, o.full); err != nil {
		return nil, err
	}
	content, err := o.content()
	if err != nil {
		return nil, err
	}
	body := map[string]any{"version": o.version}
	if content != nil {
		body["body"] = content
	}
	if o.followPage {
		body["follow_page"] = true
	}
	switch {
	case o.width != "":
		body["width"] = o.width
	case o.half:
		body["width"] = "half"
	case o.full:
		body["width"] = "full"
	}
	if cmd.Flags().Changed("height") {
		body["height"] = heightValue(o.height)
	}
	if cmd.Flags().Changed("position") {
		if o.position < 0 {
			return nil, usagef("--position must be 0 or more")
		}
		body["position"] = o.position
	}
	if len(body) == 1 {
		return nil, usagef("give something to change: --text, --text-file, --panel-file, --width, --height, --position or --follow-page")
	}
	return body, nil
}

func (o blockEditOptions) content() (any, error) {
	markdown, hasText, err := o.source.markdown()
	if err != nil {
		return nil, err
	}
	if hasText && o.panelFile != "" {
		return nil, usagef("give --text or --text-file, or --panel-file, not both")
	}
	if o.followPage && (hasText || o.panelFile != "") {
		return nil, usagef("--follow-page goes without --text, --text-file or --panel-file")
	}
	if (o.from != "" || o.to != "") && o.panelFile == "" {
		return nil, usagef("--from and --to go with --panel-file")
	}
	switch {
	case hasText:
		return map[string]string{"markdown": markdown}, nil
	case o.panelFile != "":
		return addBlockOptions{panelFile: o.panelFile, from: o.from, to: o.to}.panel()
	}
	return nil, nil
}

func (a *app) observationEditBlockCommand() *cobra.Command {
	var opts blockEditOptions
	cmd := &cobra.Command{
		Use:   "edit-block <block-id> --version <n> (--text <markdown> | --text-file <path> | --panel-file <path> | --follow-page) [--width third|half|two_thirds|full] [--height <pixels>] [--position <n>]",
		Short: "Change a live block's text, panel, width, height or position",
		Long:  "edit-block changes one live block of an observation: the Markdown of a text block, the panel of a panel block, its width, its height in pixels, or its position, which moves the blocks in between.\n--follow-page drops a panel block's own range, so it shows the observation's range again.\n--height takes 120 to 960, and 0 returns the block to its automatic height.\nTo place several blocks at once, use observe o arrange.\n--version is the block's version from observe o show; a block that changed since answers version_conflict, so read it again and retry.\nA snapshot block's body cannot change, and a requested block is kept or discarded instead.",
		Args:  exactArgs(1, "one block id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			body, err := opts.body(cmd)
			if err != nil {
				return err
			}
			return a.observationCall(cmd, http.MethodPatch, "blocks/"+id, body, func(out *output, data []byte) error {
				changed, err := decode[blockEnvelope](data)
				if err != nil {
					return err
				}
				out.confirm("Changed %s block %s", text(changed.Block.Kind), text(changed.Block.ID))
				return nil
			})
		},
	}
	flags := cmd.Flags()
	flags.IntVar(&opts.version, "version", 0, "the block's version")
	flags.StringVar(&opts.source.text, "text", "", "the new Markdown of a text block")
	flags.StringVar(&opts.source.file, "text-file", "", "read the new Markdown of a text block from this file")
	flags.StringVar(&opts.panelFile, "panel-file", "", "read the new panel of a panel block, a JSON object, from this file")
	flags.StringVar(&opts.from, "from", "", "start of the panel's own range, an RFC 3339 time")
	flags.StringVar(&opts.to, "to", "", "end of the panel's own range, an RFC 3339 time")
	flags.BoolVar(&opts.half, "half", false, "make the block half width")
	flags.BoolVar(&opts.full, "full", false, "make the block full width")
	flags.StringVar(&opts.width, "width", "", "the block's width: third, half, two_thirds or full")
	flags.IntVar(&opts.height, "height", 0, "the block's height in pixels, 120 to 960; 0 for the automatic height")
	flags.IntVar(&opts.position, "position", 0, "move the block to this position")
	flags.BoolVar(&opts.followPage, "follow-page", false, "drop a panel block's own range so it follows the observation's")
	return cmd
}

func (a *app) observationRemoveBlockCommand() *cobra.Command {
	var version int
	cmd := &cobra.Command{
		Use:   "remove-block <block-id> --version <n>",
		Short: "Remove a live block from an observation",
		Long:  "remove-block deletes one live block, and the blocks after it close the gap.\n--version is the block's version from observe o show; a block that changed since answers version_conflict.\nA requested block is kept or discarded instead.",
		Args:  exactArgs(1, "one block id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if version < 1 {
				return usagef("--version <n> is required, the block's version from observe o show")
			}
			c, err := a.newClient()
			if err != nil {
				return err
			}
			data, err := c.do(cmd.Context(), http.MethodDelete, "blocks/"+id, url.Values{"version": {strconv.Itoa(version)}}, nil)
			if err != nil {
				return err
			}
			out := a.output()
			if a.json {
				out.body(data)
				return nil
			}
			out.confirm("Removed block %s", id)
			return nil
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "the block's version")
	return cmd
}

func checkWidthFlags(width string, half, full bool) error {
	given := 0
	for _, present := range []bool{width != "", half, full} {
		if present {
			given++
		}
	}
	if given > 1 {
		return usagef("give one of --width, --half and --full")
	}
	return nil
}

func heightValue(pixels int) any {
	if pixels == 0 {
		return nil
	}
	return pixels
}

func layoutItem(arg string) (map[string]any, error) {
	parts := strings.Split(arg, ":")
	if len(parts) > 3 {
		return nil, usagef("%q is not <block-id>[:<width>[:<height>]]", text(arg))
	}
	id, err := parseID(parts[0])
	if err != nil {
		return nil, err
	}
	item := map[string]any{"id": id}
	if len(parts) > 1 && parts[1] != "" {
		item["width"] = parts[1]
	}
	if len(parts) > 2 && parts[2] != "" {
		if parts[2] == "auto" {
			item["height"] = nil
		} else {
			pixels, err := strconv.Atoi(parts[2])
			if err != nil {
				return nil, usagef("the height in %q is a number of pixels or auto", text(arg))
			}
			item["height"] = pixels
		}
	}
	return item, nil
}

func (a *app) observationArrangeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "arrange <observation-id> <block-id>[:<width>[:<height>]]...",
		Short: "Set the order, widths and heights of an observation's blocks",
		Long:  "arrange places the blocks you name in the order you name them, then the blocks you leave out keep their place after the block that precedes them.\nAfter a block id, :<width> sets third, half, two_thirds or full, and :<width>:<height> also sets the height in pixels, 120 to 960, or auto.\nLeave the width empty to keep it, as in <block-id>::auto.\nIt needs edit rights on the observation, and it never conflicts with a text edit made at the same time, so it takes no --version.",
		Args:  minimumArgs(2, "an observation id and the block ids in the order you want them"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			blocks := make([]map[string]any, 0, len(args)-1)
			for _, arg := range args[1:] {
				item, err := layoutItem(arg)
				if err != nil {
					return err
				}
				blocks = append(blocks, item)
			}
			return a.observationCall(cmd, http.MethodPut, "observations/"+id+"/layout", map[string]any{"blocks": blocks}, func(out *output, _ []byte) error {
				out.confirm("Arranged %d %s", len(blocks), plural(len(blocks), "block", "blocks"))
				return nil
			})
		},
	}
}

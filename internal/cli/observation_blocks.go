package cli

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

type blockEditOptions struct {
	source    textSource
	panelFile string
	from, to  string
	version   int
	half      bool
	full      bool
	position  int
}

func (o blockEditOptions) body(cmd *cobra.Command) (map[string]any, error) {
	if o.version < 1 {
		return nil, usagef("--version <n> is required, the block's version from observe o show")
	}
	markdown, hasText, err := o.source.markdown()
	if err != nil {
		return nil, err
	}
	if hasText && o.panelFile != "" {
		return nil, usagef("give --text or --text-file, or --panel-file, not both")
	}
	if (o.from != "" || o.to != "") && o.panelFile == "" {
		return nil, usagef("--from and --to go with --panel-file")
	}
	if o.half && o.full {
		return nil, usagef("--half and --full cannot be used together")
	}
	body := map[string]any{"version": o.version}
	switch {
	case hasText:
		body["body"] = map[string]string{"markdown": markdown}
	case o.panelFile != "":
		panel, err := addBlockOptions{panelFile: o.panelFile, from: o.from, to: o.to}.panel()
		if err != nil {
			return nil, err
		}
		body["body"] = panel
	}
	switch {
	case o.half:
		body["width"] = "half"
	case o.full:
		body["width"] = "full"
	}
	if cmd.Flags().Changed("position") {
		if o.position < 0 {
			return nil, usagef("--position must be 0 or more")
		}
		body["position"] = o.position
	}
	if len(body) == 1 {
		return nil, usagef("give something to change: --text, --text-file, --panel-file, --half, --full or --position")
	}
	return body, nil
}

func (a *app) observationEditBlockCommand() *cobra.Command {
	var opts blockEditOptions
	cmd := &cobra.Command{
		Use:   "edit-block <block-id> --version <n> (--text <markdown> | --text-file <path> | --panel-file <path>) [--half | --full] [--position <n>]",
		Short: "Change a live block's text, panel, width or position",
		Long:  "edit-block changes one live block of an observation: the Markdown of a text block, the panel of a panel block, its width, or its position, which moves the blocks in between.\n--version is the block's version from observe o show; a block that changed since answers version_conflict, so read it again and retry.\nA snapshot block's body cannot change, and a requested block is kept or discarded instead.",
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
	flags.IntVar(&opts.position, "position", 0, "move the block to this position")
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
			if _, err := c.do(cmd.Context(), http.MethodDelete, "blocks/"+id, url.Values{"version": {strconv.Itoa(version)}}, nil); err != nil {
				return err
			}
			a.output().confirm("Removed block %s", id)
			return nil
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "the block's version")
	return cmd
}

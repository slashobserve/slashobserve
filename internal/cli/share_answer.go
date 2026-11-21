package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

type sharedAnswer struct {
	ThreadID  string `json:"thread_id"`
	MessageID string `json:"message_id"`
	Replayed  bool   `json:"replayed"`
}

func (a *app) shareAnswerCommand() *cobra.Command {
	var thread string
	cmd := &cobra.Command{
		Use:   "share-answer <request-id>",
		Short: "Share the answer of a private question in a comment thread",
		Long:  "share-answer shows the answer of a private question (observe investigate --private) to everyone who can read the dashboard or observation it was about.\nIt opens a comment thread there with your question and the agent's answer, and on an observation it also writes the answer onto the page; --thread adds both to that thread instead.\nThe request and the rest of its chat stay yours. Only the person who asked can share it, and running it again prints the same thread.\nAn answer whose run read data only Admins can query, such as the audit trail, is shared only on a dashboard or observation in a folder only Admins can read; anywhere else the server refuses it with restricted_answer.",
		Args:  exactArgs(1, "one request id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runShareAnswer(cmd, args[0], thread)
		},
	}
	cmd.Flags().StringVar(&thread, "thread", "", "the id of a thread on the same dashboard or observation to add the question and answer to")
	return cmd
}

func (a *app) runShareAnswer(cmd *cobra.Command, rawID, rawThread string) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	var payload []byte
	if rawThread != "" {
		thread, err := parseID(rawThread)
		if err != nil {
			return err
		}
		if payload, err = json.Marshal(map[string]string{"thread_id": thread}); err != nil {
			return err
		}
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.do(cmd.Context(), http.MethodPost, "observe/"+id+"/share", nil, payload)
	if err != nil {
		return shareAnswerError(err, id)
	}
	shared, err := decode[sharedAnswer](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	if shared.Replayed {
		out.line("Already shared in thread %s", text(shared.ThreadID))
		return nil
	}
	out.line("Shared in thread %s", text(shared.ThreadID))
	return nil
}

func shareAnswerError(err error, id string) error {
	var api *apiError
	if !errors.As(err, &api) {
		return err
	}
	switch {
	case api.status == http.StatusConflict && api.code == "no_answer":
		return fmt.Errorf("request %s has not finished with an answer to share", id)
	case api.status == http.StatusConflict && api.code == "not_shareable":
		return fmt.Errorf("request %s is not a private question about a dashboard or an observation, so there is nothing to share", id)
	case api.status == http.StatusNotFound:
		return fmt.Errorf("request %s was not found, or it is not yours: only the person who asked can share its answer", id)
	}
	return err
}

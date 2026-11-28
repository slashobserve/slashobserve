package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

type continueOptions struct {
	note    string
	noWait  bool
	timeout time.Duration
}

type retryOptions struct {
	noWait  bool
	timeout time.Duration
}

func (a *app) cancelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <request-id>",
		Short: "Cancel a queued or running agent request",
		Long:  "cancel stops a queued or running agent request and keeps what it built so far.\nOnly the person who made the request can cancel it.",
		Args:  exactArgs(1, "one request id"),
		RunE:  a.runCancel,
	}
}

func (a *app) runCancel(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.do(cmd.Context(), http.MethodPost, "observe/"+id+"/cancel", nil, nil)
	if err != nil {
		return requestControlError(err, id)
	}
	envelope, err := decode[requestEnvelope](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	printRequest(out, envelope.Request)
	return nil
}

func (a *app) saveAnswerCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "save-answer <request-id>",
		Short: "Save the answer of an agent request as an observation",
		Long:  "save-answer files the answer a request gave to a plain question as an observation and prints its address.\nA question asked in chat or in a comment is answered on the request alone until you do this; only the person who made the request can.\nRunning it again prints the same observation.\nAn answer whose run read data only Admins can query, such as the audit trail, is saved in the folder of the dashboard or observation it was about when only Admins can read that folder, else in the built-in Audit folder; when neither is one only Admins can read, the server refuses it with restricted_answer.",
		Args:  exactArgs(1, "one request id"),
		RunE:  a.runSaveAnswer,
	}
}

func (a *app) runSaveAnswer(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.do(cmd.Context(), http.MethodPost, "observe/"+id+"/observation", nil, nil)
	if err != nil {
		return saveAnswerError(err, id)
	}
	saved, err := decode[savedAnswer](data)
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	switch {
	case saved.Replayed:
		out.line("Already an observation: %s", text(saved.URL))
	case saved.Outcome == "kept":
		out.line("Kept in the observation its thread share showed it in, so a withdraw leaves it there: %s", text(saved.URL))
	default:
		out.line("Saved as an observation: %s", text(saved.URL))
	}
	return nil
}

type savedAnswer struct {
	ObservationID string `json:"observation_id"`
	URL           string `json:"url"`
	Replayed      bool   `json:"replayed"`
	Outcome       string `json:"outcome"`
}

func saveAnswerError(err error, id string) error {
	var api *apiError
	if !errors.As(err, &api) {
		return err
	}
	switch {
	case api.status == http.StatusConflict && api.code == "no_answer":
		return fmt.Errorf("request %s has no answer to save", id)
	case api.status == http.StatusNotFound:
		return fmt.Errorf("request %s was not found, or it is not yours: only the person who made it can save its answer", id)
	case api.status == http.StatusForbidden:
		return errors.New("you may not file an observation: your role or your folders do not allow it")
	}
	return err
}

func (a *app) continueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "continue <request-id>",
		Short: "Continue a stopped agent request in a new run",
		Long:  "continue starts a new request that takes over the build of a stopped one, waits like a prompt does and prints the dashboard address.\nOnly the person who made the request can continue it.",
		Args:  exactArgs(1, "one request id"),
		RunE:  a.runContinue,
	}
	cmd.Flags().StringVar(&a.continuing.note, "note", "", "an instruction for the continued run")
	cmd.Flags().BoolVar(&a.continuing.noWait, "no-wait", false, "print the new request id and exit without waiting")
	cmd.Flags().DurationVar(&a.continuing.timeout, "timeout", defaultTimeout, "stop waiting after this long; the request keeps running on the server")
	a.addOpenFlag(cmd)
	return cmd
}

func (a *app) runContinue(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if a.continuing.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	var body []byte
	if cmd.Flags().Changed("note") {
		if body, err = json.Marshal(map[string]string{"note": a.continuing.note}); err != nil {
			return fmt.Errorf("encode the request: %w", err)
		}
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	data, err := c.do(ctx, http.MethodPost, "observe/"+id+"/continue", nil, body)
	if err != nil {
		return requestControlError(err, id)
	}
	return a.reportQueued(ctx, c, data, a.continuing.noWait, a.continuing.timeout)
}

func (a *app) retryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retry <request-id>",
		Short: "Start a finished agent request again as a new run",
		Long:  "retry starts a new request with the same prompt as a finished one, waits like a prompt does and prints the result.\nAnyone who can see the request and may start that kind of request can retry it; the new request is yours.\nComment and suggestion requests cannot be retried.",
		Args:  exactArgs(1, "one request id"),
		RunE:  a.runRetry,
	}
	cmd.Flags().BoolVar(&a.retrying.noWait, "no-wait", false, "print the new request id and exit without waiting")
	cmd.Flags().DurationVar(&a.retrying.timeout, "timeout", defaultTimeout, "stop waiting after this long; the request keeps running on the server")
	a.addOpenFlag(cmd)
	return cmd
}

func (a *app) runRetry(cmd *cobra.Command, args []string) error {
	id, err := parseID(args[0])
	if err != nil {
		return err
	}
	if a.retrying.timeout <= 0 {
		return usagef("--timeout must be positive")
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	data, err := c.do(ctx, http.MethodPost, "observe/"+id+"/retry", nil, nil)
	if err != nil {
		return retryError(err, id)
	}
	return a.reportQueued(ctx, c, data, a.retrying.noWait, a.retrying.timeout)
}

func retryError(err error, id string) error {
	var api *apiError
	if !errors.As(err, &api) {
		return err
	}
	switch {
	case api.status == http.StatusForbidden:
		return fmt.Errorf("you may not start this kind of request on the dashboard of %s", id)
	case api.status == http.StatusConflict && api.code == "invalid_transition":
		var body struct {
			State string `json:"state"`
		}
		if json.Unmarshal(api.raw, &body) == nil && body.State != "" {
			return fmt.Errorf("request %s is %s and cannot be retried", id, text(body.State))
		}
	}
	return err
}

func requestControlError(err error, id string) error {
	var api *apiError
	if !errors.As(err, &api) {
		return err
	}
	if api.status == http.StatusForbidden {
		message := "only the requester can cancel or continue " + id
		if api.code != "" {
			message += " (" + api.code + ")"
		}
		return errors.New(message)
	}
	if api.status != http.StatusConflict {
		return err
	}
	switch api.code {
	case "request_finished":
		return fmt.Errorf("request %s already finished, so it cannot be canceled", id)
	case "not_continuable":
		return fmt.Errorf("request %s has no stopped build to continue: it did not fail, was saved, was already continued or lost its build", id)
	}
	return err
}

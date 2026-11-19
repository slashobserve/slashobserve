package threads

import (
	"context"
	"errors"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/proposals"
)

func closureBody(c proposals.Closure, withReason bool) string {
	if c.State == proposals.StateMerged {
		if c.Version == nil {
			return "Merged."
		}
		return "Merged as version " + strconv.Itoa(*c.Version) + "."
	}
	if withReason && c.Reason != "" {
		return "Discarded.\n\nReason: " + c.Reason
	}
	return "Discarded."
}

func (s *Service) Closed(ctx context.Context, by auth.Principal, c proposals.Closure) error {
	in := OpenInput{Target: Target{Kind: KindProposal, ID: c.ProposalID}, Body: closureBody(c, true)}
	_, _, err := s.Open(ctx, by, in, nil)
	var invalid *InvalidInputError
	if errors.As(err, &invalid) && c.Reason != "" {
		in.Body = closureBody(c, false)
		_, _, err = s.Open(ctx, by, in, nil)
	}
	return err
}

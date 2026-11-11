package grafanaconn

import "context"

func (s *Service) ResumeOnce(ctx context.Context) { s.resumePass(ctx) }

const ExhaustedMessage = exhaustedMessage

const StarterGoneMessage = starterGoneMessage

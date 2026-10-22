package app

import (
	"nexusmail/internal/mailto"
)

// MailtoDTO is a mailto: link the launch was asked to open.
type MailtoDTO struct {
	To        string `json:"to"`
	Cc        string `json:"cc"`
	Bcc       string `json:"bcc"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	InReplyTo string `json:"inReplyTo"`
}

// PendingMailto returns the link this launch carried, exactly once.
//
// Called by the shell when it is ready rather than emitted at startup: an
// event sent before the frontend subscribed is lost, and the one thing a
// mailto handler must not do is open nothing. Taking it clears it, so a
// re-render cannot open the same draft twice.
func (s *MailService) PendingMailto() (MailtoDTO, error) {
	s.mailtoMu.Lock()
	defer s.mailtoMu.Unlock()

	draft, err := s.pendingMailto, s.mailtoErr
	s.pendingMailto = nil
	s.mailtoErr = nil
	if err != nil {
		return MailtoDTO{}, err
	}
	if draft == nil {
		return MailtoDTO{}, nil
	}
	return MailtoDTO{
		To:        draft.To,
		Cc:        draft.Cc,
		Bcc:       draft.Bcc,
		Subject:   draft.Subject,
		Body:      draft.Body,
		InReplyTo: draft.InReplyTo,
	}, nil
}

func parsePendingMailto(raw string) (*mailto.Draft, error) {
	if raw == "" {
		return nil, nil
	}
	draft, err := mailto.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

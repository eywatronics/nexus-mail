package app

import (
	"context"
	"strings"
)

// RecipientDTO is one address the composer may offer.
//
// Name is what the person signed with and may be empty; Addr is the part that
// actually has to be on the wire.
type RecipientDTO struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// SuggestRecipients completes an address from mail already on this machine.
//
// Only people who have written to this account are known: contacts are M7. The
// account's own addresses are removed, because offering somebody their own
// address is how a reply gets sent to the wrong mailbox.
func (s *MailService) SuggestRecipients(accountID int64, query string, limit int) ([]RecipientDTO, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	if strings.TrimSpace(query) == "" {
		return []RecipientDTO{}, nil
	}

	ctx := context.Background()
	acct, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	self := map[string]bool{strings.ToLower(acct.Email): true}
	if ids, err := s.store.ListIdentities(ctx, accountID); err == nil {
		for _, identity := range ids {
			self[strings.ToLower(identity.Email)] = true
		}
	}

	// Ask for a few extra so removing the account's own addresses still leaves
	// a full list. The cap keeps one query from reading the whole mailbox.
	fetch := limit + len(self)
	if fetch > 40 {
		fetch = 40
	}
	addrs, err := s.store.SuggestRecipients(ctx, accountID, query, fetch)
	if err != nil {
		return nil, err
	}

	out := make([]RecipientDTO, 0, limit)
	for _, a := range addrs {
		if self[strings.ToLower(a.Addr)] {
			continue
		}
		out = append(out, RecipientDTO{Name: a.Name, Addr: a.Addr})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

package app

import "nexusmail/internal/model"

// AccountDTO is what the UI sees.
//
// SecretRef is deliberately absent: the frontend has no business knowing where
// credentials live, and a field that never crosses the bridge cannot leak
// through a console log or a devtools inspection.
type AccountDTO struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Provider    string `json:"provider"`
	AuthKind    string `json:"authKind"`
}

type FolderDTO struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"accountId"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	TotalCount  int    `json:"totalCount"`
	UnreadCount int    `json:"unreadCount"`
	IsInbox     bool   `json:"isInbox"`
}

type MessageDTO struct {
	ID       int64  `json:"id"`
	FolderID int64  `json:"folderId"`
	UID      uint32 `json:"uid"`
	ThreadID string `json:"threadId"`
	Subject  string `json:"subject"`
	FromName string `json:"fromName"`
	FromAddr string `json:"fromAddr"`
	Snippet  string `json:"snippet"`
	// InternalDateUnix is seconds since the epoch. An explicit integer avoids
	// the timezone ambiguity that string dates cause across the JS boundary,
	// and it is the server's delivery time rather than the forgeable Date:
	// header.
	InternalDateUnix int64 `json:"internalDateUnix"`
	IsRead           bool  `json:"isRead"`
	IsStarred        bool  `json:"isStarred"`
	HasAttachments   bool  `json:"hasAttachments"`
	BodyFetched      bool  `json:"bodyFetched"`
}

func accountToDTO(a model.Account) AccountDTO {
	return AccountDTO{
		ID:          a.ID,
		Email:       a.Email,
		DisplayName: a.DisplayName,
		Provider:    string(a.Provider),
		AuthKind:    string(a.AuthKind),
	}
}

func folderToDTO(f model.Folder) FolderDTO {
	return FolderDTO{
		ID:          f.ID,
		AccountID:   f.AccountID,
		Name:        f.Name,
		Path:        f.Path,
		TotalCount:  f.TotalCount,
		UnreadCount: f.UnreadCount,
		IsInbox:     f.IsInbox(),
	}
}

func messageToDTO(m model.Message) MessageDTO {
	return MessageDTO{
		ID:               m.ID,
		FolderID:         m.FolderID,
		UID:              m.UID,
		ThreadID:         m.ThreadID,
		Subject:          m.Subject,
		FromName:         m.From.Name,
		FromAddr:         m.From.Addr,
		Snippet:          m.Snippet,
		InternalDateUnix: m.InternalDate.Unix(),
		IsRead:           m.HasFlag(model.FlagSeen),
		IsStarred:        m.HasFlag(model.FlagFlagged),
		HasAttachments:   m.HasAttachments,
		BodyFetched:      m.BodyFetched,
	}
}

package app

import (
	"context"
	"fmt"

	"nexusmail/internal/imapx"
)

// CharsetDTO is one entry in the encoding picker.
type CharsetDTO struct {
	// Name is the IANA name, which is what the repair is performed with.
	Name string `json:"name"`
	// Label names the languages the encoding is used for, because that is what
	// a reader knows about their own mail. Almost nobody looking at a mangled
	// message knows it is Latin-5; plenty of them know it is Turkish.
	Label string `json:"label"`
}

// charsetLabels are the human names for the encodings the repair offers. The
// list itself lives next to the decoder in imapx; only the wording is here.
var charsetLabels = map[string]string{
	"utf-8":        "Unicode (UTF-8)",
	"iso-8859-9":   "Turkish (ISO-8859-9)",
	"windows-1254": "Turkish (Windows-1254)",
	"iso-8859-1":   "Western European (ISO-8859-1)",
	"windows-1252": "Western European (Windows-1252)",
	"iso-8859-2":   "Central European (ISO-8859-2)",
	"windows-1250": "Central European (Windows-1250)",
	"windows-1251": "Cyrillic (Windows-1251)",
}

// RepairCharsets returns the encodings a message can be re-read with.
func (s *MailService) RepairCharsets() []CharsetDTO {
	out := make([]CharsetDTO, 0, len(imapx.RepairCharsets))
	for _, name := range imapx.RepairCharsets {
		label := charsetLabels[name]
		if label == "" {
			label = name
		}
		out = append(out, CharsetDTO{Name: name, Label: label})
	}
	return out
}

// RepairEncoding re-reads a message with a character set the reader chose, and
// replaces the cached body with the result.
//
// The choice is stored rather than applied for one render. A reader who has
// worked out that a correspondent's system sends Latin-5 under a UTF-8 header
// should not have to work it out again every time they open the same message —
// and because bodies are only fetched when the cache is empty, the correction
// survives every later sync.
//
// The rendered-HTML cache has to be dropped as well. It is keyed on message id
// and knows nothing about encodings, so without this the reading pane would go
// on serving the mojibake it had already sanitised.
func (s *MailService) RepairEncoding(messageID int64, charsetName string) error {
	ctx := context.Background()

	raw, err := s.rawMessage(ctx, messageID)
	if err != nil {
		return err
	}

	body, err := imapx.DecodeBodyForcingCharset(raw, charsetName)
	if err != nil {
		return err
	}
	if err := s.store.SetMessageBody(ctx, messageID, body.HTML, body.Text); err != nil {
		return fmt.Errorf("app: storing the repaired body: %w", err)
	}

	if s.cfg.InvalidateBody != nil {
		s.cfg.InvalidateBody(messageID)
	}
	return nil
}

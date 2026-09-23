package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"nexusmail/internal/model"
)

// SearchMessages finds an account's messages by subject, sender address or
// snippet.
//
// The query goes through the FTS5 index as a subquery rather than a join. The
// index shares three column names with `messages`, so a join would need every
// selected column qualified and would silently return the index's copy of a
// value the day someone forgets a prefix. `id IN (SELECT rowid ...)` keeps the
// column list identical to every other message query in this file.
//
// Results are ordered by date rather than FTS5 rank. Someone searching their
// mail is nearly always looking for something recent, and relevance ranking
// over three short columns mostly reorders by which field matched.
//
// Bodies are not searched. They are fetched lazily, so a body index would be
// silently incomplete — it would find the messages that happen to have been
// opened and quietly miss the rest, which is worse than not offering it. P3
// adds it with a backfill.
func (s *Store) SearchMessages(ctx context.Context, accountID int64, query string, limit int) ([]model.Message, error) {
	match := ftsQuery(query)
	if match == "" {
		// Nothing searchable in the input. Returning no results beats handing
		// FTS5 an empty MATCH, which is a syntax error.
		return nil, nil
	}

	rows, err := s.read.QueryContext(ctx,
		`SELECT `+messageColumns+`
		 FROM messages
		 WHERE account_id = ?
		   AND id IN (SELECT rowid FROM fts_messages WHERE fts_messages MATCH ?)
		 ORDER BY internal_date DESC, uid DESC
		 LIMIT ?`, accountID, match, limit)
	if err != nil {
		// The query text is deliberately absent from this error: it is
		// something the user typed, and it ends up in the log.
		return nil, fmt.Errorf("store: search messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanMessageRows(rows)
}

// ftsQuery turns what someone typed into an FTS5 MATCH expression.
//
// FTS5 has a grammar of its own — column filters (`subject:x`), the boolean
// operators AND/OR/NOT, NEAR, phrase quoting, prefix stars, initial-token
// carets. A search box does not. Passing the raw text through means a colon or
// an unpaired quote returns a syntax error where the user expected results,
// and the word "AND" typed in a sentence silently changes what was asked.
//
// So the input is reduced to its letters and digits: everything else is a
// separator. Because a term can then never contain a quote, wrapping each one
// in quotes is enough to make it a literal, with no escaping left to get
// wrong. Terms are joined by implicit AND — all of them must appear.
//
// Only the last term gets a prefix star, so results narrow as someone types
// the word they are on. Starring every term would make "top not" match
// "topluluk notları", which is not what was asked.
//
// Returns "" when there is nothing to search for; the caller returns no
// results rather than passing an empty expression to MATCH.
func ftsQuery(input string) string {
	var (
		terms []string
		term  strings.Builder
	)
	flush := func() {
		if term.Len() > 0 {
			terms = append(terms, term.String())
			term.Reset()
		}
	}

	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			term.WriteRune(r)
			continue
		}
		flush()
	}
	flush()

	if len(terms) == 0 {
		return ""
	}

	parts := make([]string, len(terms))
	for i, term := range terms {
		star := ""
		if i == len(terms)-1 {
			star = "*"
		}

		variants := dotlessIVariants(term)
		if len(variants) == 1 {
			parts[i] = `"` + variants[0] + `"` + star
			continue
		}

		alts := make([]string, len(variants))
		for j, v := range variants {
			alts[j] = `"` + v + `"` + star
		}
		parts[i] = "(" + strings.Join(alts, " OR ") + ")"
	}

	// The AND is explicit rather than relying on juxtaposition. FTS5 treats two
	// adjacent phrases as an implicit AND, but that shorthand does not extend to
	// a parenthesised group: `("a" OR "b") "c"` is a syntax error, so the moment
	// a term expands the whole expression stops parsing. Spelling out AND costs
	// four characters and works in every combination.
	return strings.Join(parts, " AND ")
}

// maxDotlessIVariants caps the expansion below. Each i in a term doubles the
// number of alternatives, and a Turkish word like "iyileştirilmiş" carries
// five. Past the cap the term is searched as typed: someone who typed a word
// that long typed its dots too, and a query built from dozens of alternatives
// is a worse answer than a narrow one.
const maxDotlessIVariants = 16

// dotlessIVariants expands the dotted/dotless i so a term matches both.
//
// FTS5's unicode61 tokenizer folds diacritics, and for Turkish it gets almost
// everything right on its own: Ş folds to s, Ğ to g, İ to i, so "subat" finds
// "Şubat" and "istanbul" finds "İstanbul" without any help. Measured, not
// assumed.
//
// The exception is ı (U+0131). It is not an i with something removed; it is a
// separate letter, and nothing folds the two together. So "mutabakati" typed
// on a keyboard without Turkish layout finds nothing, while "mutabakatı" typed
// on one with it finds everything — the same search working for one half of
// the intended users and silently failing for the other.
//
// Every position holding one of i ı I İ becomes a choice between the two
// letters, and the caller joins the results with OR.
//
// The four runes are matched literally rather than by lowercasing the term
// first. strings.ToLower("İ") returns i followed by a combining dot above —
// two runes where there was one — and that combining mark would travel into
// the query. This project has already been bitten by exactly that once.
func dotlessIVariants(term string) []string {
	runes := []rune(term)

	var positions []int
	for i, r := range runes {
		switch r {
		case 'i', 'ı', 'I', 'İ':
			positions = append(positions, i)
		}
	}
	if len(positions) == 0 {
		return []string{term}
	}
	if 1<<len(positions) > maxDotlessIVariants {
		return []string{term}
	}

	out := make([]string, 0, 1<<len(positions))
	for mask := 0; mask < 1<<len(positions); mask++ {
		variant := make([]rune, len(runes))
		copy(variant, runes)
		for bit, pos := range positions {
			if mask&(1<<bit) != 0 {
				variant[pos] = 'ı'
			} else {
				variant[pos] = 'i'
			}
		}
		out = append(out, string(variant))
	}
	return out
}

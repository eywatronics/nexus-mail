package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// identityFixture gives a store with one account and the identity the
// migration gave it.
func identityFixture(t *testing.T) (*Store, int64) {
	t.Helper()

	s := openTestStore(t)
	id, err := s.InsertAccount(context.Background(), model.Account{
		Email: "u@example.com", DisplayName: "Yazan",
		Provider: model.ProviderGeneric, AuthKind: model.AuthPassword,
		IMAPHost: "imap.example.com", IMAPPort: 993,
		SecretRef: "ref", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}
	return s, id
}

// An account with no identity cannot open a composer, so the first one takes
// the role whatever the caller asked for.
func TestTheFirstIdentityBecomesTheDefault(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	// The fixture's account was created after the migration, so it starts with
	// none: the caller adds the first.
	if _, err := s.InsertIdentity(ctx, model.Identity{
		AccountID: account, Email: "u@example.com", IsDefault: false,
	}); err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}

	got, err := s.DefaultIdentity(ctx, account)
	if err != nil {
		t.Fatalf("DefaultIdentity() error: %v", err)
	}
	if got.Email != "u@example.com" {
		t.Errorf("the default is %q", got.Email)
	}
	if !got.IsDefault {
		t.Error("the first identity did not take the default role")
	}
}

// A mailbox with two defaults has no answer to "who is this from". The schema
// refuses it outright rather than leaving it to application code.
func TestAnAccountNeverHasTwoDefaults(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	for _, email := range []string{"u@example.com", "support@example.com", "sales@example.com"} {
		if _, err := s.InsertIdentity(ctx, model.Identity{
			AccountID: account, Email: email, IsDefault: true,
		}); err != nil {
			t.Fatalf("InsertIdentity(%s) error: %v", email, err)
		}
	}

	var defaults int
	if err := s.Read().QueryRow(
		`SELECT count(*) FROM identities WHERE account_id = ? AND is_default = 1`,
		account).Scan(&defaults); err != nil {
		t.Fatalf("counting defaults: %v", err)
	}
	if defaults != 1 {
		t.Errorf("%d identities claim the default", defaults)
	}

	// And it is the last one asked for.
	got, _ := s.DefaultIdentity(ctx, account)
	if got.Email != "sales@example.com" {
		t.Errorf("the default is %q, want the one most recently asked for", got.Email)
	}
}

// The same address twice on one account is a mistake; the same address on two
// accounts is a shared alias delivered to two mailboxes.
func TestAnAddressIsUniquePerAccountAndNotAcrossThem(t *testing.T) {
	s, first := identityFixture(t)
	ctx := context.Background()

	if _, err := s.InsertIdentity(ctx, model.Identity{
		AccountID: first, Email: "shared@example.com",
	}); err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}
	if _, err := s.InsertIdentity(ctx, model.Identity{
		AccountID: first, Email: "shared@example.com",
	}); err == nil {
		t.Error("the same address was added twice to one account")
	}

	second, err := s.InsertAccount(ctx, model.Account{
		Email: "other@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "ref2", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}
	if _, err := s.InsertIdentity(ctx, model.Identity{
		AccountID: second, Email: "shared@example.com",
	}); err != nil {
		t.Errorf("a shared alias was refused on a second account: %v", err)
	}
}

// Clearing and setting in two statements would leave a moment with no default,
// and a composer opened then has nothing to send as.
func TestMovingTheDefaultLeavesExactlyOne(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	first, _ := s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "a@example.com"})
	second, _ := s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "b@example.com"})

	if err := s.SetDefaultIdentity(ctx, second); err != nil {
		t.Fatalf("SetDefaultIdentity() error: %v", err)
	}
	got, err := s.DefaultIdentity(ctx, account)
	if err != nil {
		t.Fatalf("DefaultIdentity() error: %v", err)
	}
	if got.ID != second {
		t.Errorf("the default is %d, want %d", got.ID, second)
	}

	if err := s.SetDefaultIdentity(ctx, first); err != nil {
		t.Fatalf("moving it back: %v", err)
	}
	if got, _ := s.DefaultIdentity(ctx, account); got.ID != first {
		t.Errorf("the default did not move back")
	}
}

func TestSettingTheDefaultToSomethingThatDoesNotExistIsReported(t *testing.T) {
	s, _ := identityFixture(t)

	err := s.SetDefaultIdentity(context.Background(), 9999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("SetDefaultIdentity() error = %v, want ErrNotFound", err)
	}
}

// An account with no identity cannot send, and the error says so rather than
// leaving the caller to find out later.
func TestTheLastIdentityCannotBeDeleted(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	only, _ := s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "a@example.com"})

	err := s.DeleteIdentity(ctx, only)
	if err == nil {
		t.Fatal("the only identity was deleted")
	}
	if !strings.Contains(err.Error(), "cannot send") {
		t.Errorf("the error does not explain why: %v", err)
	}
}

// Leaving an account with identities and no default is the same broken state
// by another route.
func TestDeletingTheDefaultHandsTheRoleOn(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	// The order the user arranged, not the order they were added.
	first, _ := s.InsertIdentity(ctx, model.Identity{
		AccountID: account, Email: "a@example.com", SortOrder: 2, IsDefault: true})
	_, _ = s.InsertIdentity(ctx, model.Identity{
		AccountID: account, Email: "b@example.com", SortOrder: 1})

	if err := s.DeleteIdentity(ctx, first); err != nil {
		t.Fatalf("DeleteIdentity() error: %v", err)
	}

	got, err := s.DefaultIdentity(ctx, account)
	if err != nil {
		t.Fatalf("the account was left with no default: %v", err)
	}
	if got.Email != "b@example.com" {
		t.Errorf("the role went to %q", got.Email)
	}
}

func TestAnIdentitySurvivesARoundTrip(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	want := model.Identity{
		AccountID:     account,
		Email:         "destek@example.com",
		DisplayName:   "Destek Ekibi",
		ReplyTo:       "yanit@example.com",
		SignatureText: "İyi çalışmalar",
		SignatureHTML: "<p>İyi çalışmalar</p>",
		SortOrder:     3,
	}
	id, err := s.InsertIdentity(ctx, want)
	if err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}

	list, err := s.ListIdentities(ctx, account)
	if err != nil {
		t.Fatalf("ListIdentities() error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListIdentities() returned %d", len(list))
	}

	got := list[0]
	if got.ID != id || got.Email != want.Email || got.DisplayName != want.DisplayName ||
		got.ReplyTo != want.ReplyTo || got.SignatureText != want.SignatureText ||
		got.SignatureHTML != want.SignatureHTML || got.SortOrder != want.SortOrder {
		t.Errorf("the identity came back as %+v", got)
	}
}

// The user arranged the list; the arrangement is the answer.
func TestIdentitiesComeBackDefaultFirstThenInTheChosenOrder(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	_, _ = s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "c@example.com", SortOrder: 3})
	_, _ = s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "a@example.com", SortOrder: 1})
	last, _ := s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "b@example.com", SortOrder: 2})

	if err := s.SetDefaultIdentity(ctx, last); err != nil {
		t.Fatalf("SetDefaultIdentity() error: %v", err)
	}

	list, _ := s.ListIdentities(ctx, account)
	var order []string
	for _, i := range list {
		order = append(order, i.Email)
	}
	want := "b@example.com,a@example.com,c@example.com"
	if strings.Join(order, ",") != want {
		t.Errorf("order = %v, want the default first then sort_order", order)
	}
}

func TestUpdateChangesWhatItShouldAndNothingElse(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	id, _ := s.InsertIdentity(ctx, model.Identity{
		AccountID: account, Email: "a@example.com", DisplayName: "Eski"})

	if err := s.UpdateIdentity(ctx, model.Identity{
		ID: id, Email: "a@example.com", DisplayName: "Yeni", SignatureText: "imza"}); err != nil {
		t.Fatalf("UpdateIdentity() error: %v", err)
	}

	got, _ := s.DefaultIdentity(ctx, account)
	if got.DisplayName != "Yeni" || got.SignatureText != "imza" {
		t.Errorf("the update did not land: %+v", got)
	}
	// The default role is moved by SetDefaultIdentity, not by an edit.
	if !got.IsDefault {
		t.Error("an edit cleared the default role")
	}
	if got.AccountID != account {
		t.Error("an edit moved the identity to another account")
	}
}

func TestAnIdentityNeedsAnAddress(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	if _, err := s.InsertIdentity(ctx, model.Identity{AccountID: account}); err == nil {
		t.Error("InsertIdentity() accepted an identity with no address")
	}
	if err := s.UpdateIdentity(ctx, model.Identity{ID: 1}); err == nil {
		t.Error("UpdateIdentity() accepted an identity with no address")
	}
}

// The From header is where the identity is actually seen.
func TestFromReadsAsAMailAddress(t *testing.T) {
	bare := model.Identity{Email: "u@example.com"}
	if got := bare.From(); got != "u@example.com" {
		t.Errorf("From() = %q for an identity with no display name", got)
	}

	named := model.Identity{Email: "u@example.com", DisplayName: "Yazan"}
	if got := named.From(); got != "Yazan <u@example.com>" {
		t.Errorf("From() = %q", got)
	}
}

// Deleting an account takes its identities with it; a row pointing at an
// account that is gone is a row nothing can ever reach.
func TestIdentitiesGoWithTheirAccount(t *testing.T) {
	s, account := identityFixture(t)
	ctx := context.Background()

	_, _ = s.InsertIdentity(ctx, model.Identity{AccountID: account, Email: "a@example.com"})

	if _, err := s.Write().ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, account); err != nil {
		t.Fatalf("deleting the account: %v", err)
	}

	var remaining int
	if err := s.Read().QueryRow(`SELECT count(*) FROM identities`).Scan(&remaining); err != nil {
		t.Fatalf("counting identities: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d identities outlived their account", remaining)
	}
}

// The migration has to give every account that already existed the identity it
// was already using.
//
// Without it, a database created before migration 004 comes back with accounts
// that have no identity at all — and an account with none cannot open a
// composer. Simulated by winding user_version back and dropping the table,
// which is what such a database looks like.
func TestTheMigrationGivesExistingAccountsTheIdentityTheyHad(t *testing.T) {
	s := openTestStore(t)
	db := s.Write()

	// Wind the schema back to before identities existed.
	if _, err := db.Exec(`DROP TABLE identities`); err != nil {
		t.Fatalf("dropping the table: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatalf("winding user_version back: %v", err)
	}

	// An account as it would have been left by the older schema.
	if _, err := db.Exec(
		`INSERT INTO accounts (email, display_name, provider, auth_kind,
		   imap_host, imap_port, imap_security, secret_ref, created_at)
		 VALUES ('eski@example.com', 'Eski Hesap', 'generic', 'password',
		   'imap.example.com', 993, 'tls', 'ref', 1700000000)`); err != nil {
		t.Fatalf("inserting the old account: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate() error: %v", err)
	}

	var accountID int64
	if err := db.QueryRow(
		`SELECT id FROM accounts WHERE email = 'eski@example.com'`).Scan(&accountID); err != nil {
		t.Fatalf("finding the account: %v", err)
	}

	got, err := s.DefaultIdentity(context.Background(), accountID)
	if err != nil {
		t.Fatalf("the migrated account has no default identity: %v", err)
	}
	if got.Email != "eski@example.com" {
		t.Errorf("the identity's address is %q", got.Email)
	}
	if got.DisplayName != "Eski Hesap" {
		t.Errorf("the identity's display name is %q; the account's was not carried over",
			got.DisplayName)
	}

	// Exactly one, not one per run of the migration.
	var count int
	if err := db.QueryRow(
		`SELECT count(*) FROM identities WHERE account_id = ?`, accountID).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 1 {
		t.Errorf("the account got %d identities", count)
	}
}

package store

import (
	"path/filepath"
	"testing"
)

func TestDedupeNormalizedEmails(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_, err = s.db.Exec(`INSERT INTO emails (address, status) VALUES
		('User@Example.com', 'pending'),
		('"user@example.com"', 'sent'),
		('other@x.co', 'pending')`)
	if err != nil {
		t.Fatal(err)
	}

	removed, err := s.DedupeNormalizedEmails()
	if err != nil {
		t.Fatal(err)
	}
	if removed < 1 {
		t.Fatalf("expected at least 1 removed, got %d", removed)
	}

	page, err := s.ListEmailsPage("all", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("total=%d want 2 (deduped example.com + other)", page.Total)
	}
	var foundSent bool
	for _, e := range page.Items {
		if e.Address == "user@example.com" && e.Status == "sent" {
			foundSent = true
		}
	}
	if !foundSent {
		t.Fatalf("expected canonical sent user@example.com, got %#v", page.Items)
	}
}

func TestListEmailsPageSearch(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ins, skip, err := s.ImportEmails([]string{"a@x.com", "b@y.com", "a@x.com"})
	if err != nil {
		t.Fatal(err)
	}
	if ins != 2 || skip != 1 {
		t.Fatalf("ins=%d skip=%d", ins, skip)
	}

	page, err := s.ListEmailsPage("all", "y.com", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Address != "b@y.com" {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestEmailCountsTrackImportAndClaim(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ins, _, err := s.ImportEmails([]string{"a@x.com", "b@y.com", "c@z.com"})
	if err != nil || ins != 3 {
		t.Fatalf("import ins=%d err=%v", ins, err)
	}
	st, err := s.GetStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending != 3 {
		t.Fatalf("pending=%d want 3", st.Pending)
	}

	e, err := s.ClaimPendingEmail()
	if err != nil {
		t.Fatal(err)
	}
	st, _ = s.GetStatus()
	if st.Pending != 2 || st.Sending != 1 {
		t.Fatalf("after claim pending=%d sending=%d", st.Pending, st.Sending)
	}
	if err := s.MarkEmailSent(e.ID, 0, "s", "http://x"); err != nil {
		t.Fatal(err)
	}
	st, _ = s.GetStatus()
	if st.Pending != 2 || st.Sending != 0 || st.Sent != 1 {
		t.Fatalf("after sent %#v", st)
	}
}

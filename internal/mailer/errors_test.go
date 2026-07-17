package mailer_test

import (
	"errors"
	"testing"

	"github.com/wiz/sendsmtp/internal/mailer"
)

func TestClassifyRecipientDoesNotPenalizeSMTP(t *testing.T) {
	err := errors.New("rcpt: 550 The mail server could not deliver mail to user@example.com")
	if mailer.ShouldPenalizeSMTP(err) {
		t.Fatal("550 recipient should not penalize SMTP")
	}
	if mailer.ShouldRetryEmail(err, 1, 3) {
		t.Fatal("550 recipient should not retry")
	}
}

func TestClassifyTimeoutPenalizes(t *testing.T) {
	err := errors.New("tls dial: dial tcp 1.2.3.4:465: i/o timeout")
	if !mailer.ShouldPenalizeSMTP(err) {
		t.Fatal("timeout should penalize SMTP")
	}
}

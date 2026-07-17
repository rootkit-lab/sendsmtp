package mailer

import (
	"strings"
)

// ErrorClass separates recipient problems from SMTP/transport failures.
type ErrorClass int

const (
	ErrorTransient ErrorClass = iota
	ErrorPermanentRecipient
	ErrorSMTPFatal
)

// ClassifyError decides whether the SMTP account should be penalized.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorTransient
	}
	msg := strings.ToLower(err.Error())

	// Bad / unknown recipient — do not disable the SMTP.
	if strings.Contains(msg, "rcpt:") {
		if hasCode(msg, "550", "551", "552", "553", "501", "503", "521", "554") {
			return ErrorPermanentRecipient
		}
		if strings.Contains(msg, "user unknown") ||
			strings.Contains(msg, "mailbox unavailable") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "bad recipient") ||
			strings.Contains(msg, "invalid recipient") ||
			strings.Contains(msg, "no such user") {
			return ErrorPermanentRecipient
		}
	}

	// Auth / account level problems — disable SMTP faster.
	if strings.Contains(msg, "auth:") ||
		strings.Contains(msg, "535") ||
		strings.Contains(msg, "534") ||
		strings.Contains(msg, "authentication failed") {
		return ErrorSMTPFatal
	}

	return ErrorTransient
}

func hasCode(msg string, codes ...string) bool {
	for _, c := range codes {
		if strings.Contains(msg, c) {
			return true
		}
	}
	return false
}

// ShouldRetryEmail is false for permanent recipient rejects.
func ShouldRetryEmail(err error, attempts, retryMax int) bool {
	if ClassifyError(err) == ErrorPermanentRecipient {
		return false
	}
	return attempts < retryMax
}

// ShouldPenalizeSMTP is false for recipient-only failures.
func ShouldPenalizeSMTP(err error) bool {
	switch ClassifyError(err) {
	case ErrorPermanentRecipient:
		return false
	default:
		return true
	}
}

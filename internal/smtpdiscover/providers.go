package smtpdiscover

import "strings"

// submissionHostsFromMX maps inbound MX fingerprints → outbound SMTP submission hosts.
// Custom domains (e.g. @azzultech.com.br on Locaweb) never match knownProviders by email domain.
func submissionHostsFromMX(mxHosts []string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	joined := strings.Join(mxHosts, " ")
	switch {
	case strings.Contains(joined, "locaweb.com.br") || strings.Contains(joined, "email-ssl.com.br"):
		// Email Pro / Hospedagem — TLS cert is *.email-ssl.com.br (smtp.customer.tld fails verify).
		add("email-ssl.com.br")
		add("smtp.locaweb.com.br")
	case strings.Contains(joined, "google.com") || strings.Contains(joined, "googlemail.com"):
		add("smtp.gmail.com")
	case strings.Contains(joined, "outlook.com") || strings.Contains(joined, "protection.outlook.com"):
		add("smtp.office365.com")
	case strings.Contains(joined, "zoho.com") || strings.Contains(joined, "zoho.eu"):
		add("smtp.zoho.com")
	case strings.Contains(joined, "secureserver.net"):
		add("smtpout.secureserver.net")
	case strings.Contains(joined, "yahoodns.net") || strings.Contains(joined, "yahoo.com"):
		add("smtp.mail.yahoo.com")
	case strings.Contains(joined, "kinghost.net") || strings.Contains(joined, "kinghost.com.br"):
		add("smtp.kinghost.net")
	case strings.Contains(joined, "uai.com.br"):
		add("smtp.uai.com.br")
	}
	return out
}

// isInboundMXHost skips relays that accept inbound mail but not client AUTH submission.
func isInboundMXHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if strings.HasPrefix(h, "mx.") || strings.HasPrefix(h, "mx1.") || strings.HasPrefix(h, "mx2.") ||
		strings.HasPrefix(h, "mxa.") || strings.HasPrefix(h, "mxb.") {
		return true
	}
	if strings.Contains(h, "mail.protection.outlook.com") {
		return true
	}
	if strings.Contains(h, "aspmx.l.google") || strings.Contains(h, "alt1.aspmx") || strings.Contains(h, "alt2.aspmx") {
		return true
	}
	if strings.Contains(h, "locaweb.com.br") && strings.Contains(h, "mx.") {
		return true
	}
	return false
}

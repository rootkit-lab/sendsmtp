Help the user add SMTP accounts for SendSMTP.

## Format A — `email;password` (auto-discover)

```
user@company.com;SecretPass
billing@example.com;AnotherPass
```

On import, SendSMTP discovers the SMTP host/port (known providers, MX fingerprint, smtp./mail.), probes AUTH, and saves working accounts only.

## Format B — goscan (explicit host)

```
--- SMTP config (goscan) ---
domain: example.com
account_label: MAIL/SMTP
host: mail.example.com
port: 587
encryption: tls
from: info@example.com
user: info@example.com
password: secret
```

Notes:

- Multiple blocks/lines can be in the same paste.
- `encryption`: `tls` (STARTTLS on 587, implicit TLS on 465), `starttls`, `ssl`, or `none`.
- `from` and `user` must be real emails — do **not** use `${MAIL_USERNAME}` or similar.
- After pasting, import via the UI or `./bin/sendsmtp-cli import smtps`.
- Never commit real passwords; `data/smtps.txt` is gitignored.
- Locaweb custom domains typically submit via `email-ssl.com.br` (detected from MX).

Help the user add SMTP accounts for SendSMTP.

## Format A — email;senha (auto-discover)

```
atendimento@creluz.com.br;@Creluz2026
outro@empresa.com;senha123
```

On import, SendSMTP discovers SMTP host/port (smtp./mail./providers/MX), probes AUTH, and saves working accounts.

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
- After pasting, import via UI button or `./bin/sendsmtp-cli import smtps`.
- Never commit real passwords; `data/smtps.txt` is gitignored.

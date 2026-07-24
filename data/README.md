# data/

Local content and examples. Real paths come from `app.config.yml`.

| File | Purpose |
|------|---------|
| `msg.html` | Campaign HTML body |
| `msg.example.html` | Same template for setup (`cp`) |
| `assuntos.txt` | Subject lines (1/line), spintax + `{{uniq}}` |
| `assuntos.example.txt` | Example subjects |
| `links.txt` | URLs (1/line) |
| `emails.txt` | Recipients (1/line) — large lists: UI **Import file** |
| `smtps.txt` | Goscan blocks **or** `email;password` (**do not commit**) |
| `sendsmtp.db` | SQLite (**do not commit**) |
| `extracted/` | IMAP extract output (**do not commit**) |

Optional import validation: syntax + DNS MX + disposable blocklist; invalid/duplicate addresses are skipped.

**Extract contacts** (SMTPs page): writes under `extracted/` **and** imports into the Emails queue automatically.

## Placeholders

`{{email}}` `{{link}}` `{{assunto}}` `{{subject}}` `{{from}}` `{{uniq}}` `{{id}}` `{{data}}` / `{{date}}` (DD/MM/YYYY, America/Sao_Paulo)

On send, `{{link}}` gets `?p=<recipient>`:

`https://example.com/` → `https://example.com/?p=user%40gmail.com`

Do not add `?p=` in the HTML or in `links.txt` — `mailer.PersonalizeLink` handles it.

## Spintax

`{a|b|c}` — pipe required. `{Status}` alone does **not** spin.

## From in the footer

```html
{Automatic notice|Auto receipt}<span data-from>{{from}}</span> · {{uniq}}
```

If the SMTP From is `${MAIL_USERNAME}` or empty, that span is omitted. Set `from:` in goscan to a real email address.

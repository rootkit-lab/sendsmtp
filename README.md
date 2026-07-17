# SendSMTP

Desktop SMTP campaign tool built with **[Wails v3](https://v3.wails.io/)**, **React/TypeScript**, Tailwind, a shared **Go** engine, and **SQLite**.

The UI (`app.go`) and CLI (`cmd/sendsmtp`) share the same packages under `internal/`.

## Features

- Import SMTPs as `email;password` (auto host discovery + AUTH) or goscan blocks
- MX-based provider detection (Locaweb → `email-ssl.com.br`, Google Workspace, Microsoft 365, …)
- Large recipient lists via **file import** (avoids Wails IPC paste limits)
- Optional validation: syntax + real MX + disposable blocklist
- IMAP contact extract → auto-adds to the email queue; credentials become SMTPs
- HTML templates with placeholders, spintax `{a|b|c}`, and personalized `{{link}}?p=<email>`
- Dashboard with cached status counters (scales to large queues)
- MailReach free inbox/spam placement check

## Requirements

| Tool | Notes |
|------|--------|
| Go 1.25+ | Module toolchain may auto-download |
| Node.js 20+ | Frontend |
| Wails v3 CLI | `wails3` |
| Linux / macOS / Windows | Per Wails platform support (Linux needs WebKitGTK) |

## Install (release)

### Windows / one-shot `.deb`

Download from [Releases](https://github.com/rootkit-lab/sendsmtp/releases):

| Platform | Asset |
|----------|--------|
| Linux (Debian/Ubuntu) | `sendsmtp_*_amd64.deb` |
| Windows | `sendsmtp_*_amd64.msi` |

```bash
# Debian / Ubuntu (local file)
sudo apt install ./sendsmtp_*_amd64.deb

# Windows — double-click the .msi, or:
msiexec /i sendsmtp_*_amd64.msi
```

### Linux via APT (signed — Cloudsmith)

After the Cloudsmith open-source repo is configured (see below), users install with:

```bash
curl -sLf 'https://dl.cloudsmith.io/public/rootkit-lab/sendsmtp/cfg/setup/bash.deb.sh' | sudo bash
sudo apt update
sudo apt install sendsmtp
```

The setup script installs Cloudsmith’s **GPG-signed** APT source. Package metadata is signed by Cloudsmith; you do not manage GPG keys yourself.

Tagged releases build `.deb` + `.msi` and (when `CLOUDSMITH_API_KEY` is set) push the `.deb` to Cloudsmith.

#### Maintainer setup (one time)

1. Create a free account at [cloudsmith.com](https://cloudsmith.com) and a **public open-source** Debian repository named `sendsmtp` under namespace `rootkit-lab` (or set GitHub Actions variables `CLOUDSMITH_OWNER` / `CLOUDSMITH_REPO`).
2. Create an API key (Cloudsmith → API Keys) with push permission.
3. In the GitHub repo: **Settings → Secrets and variables → Actions** → secret `CLOUDSMITH_API_KEY`.
4. Push a tag `v*` (or re-run the Release workflow) — the `.deb` is published automatically.
5. Attribute Cloudsmith hosting in docs (required by their [OSS policy](https://docs.cloudsmith.com/open-source-hosting-policy)).

APT packages for SendSMTP are hosted by [Cloudsmith](https://cloudsmith.com).

## Quick start (from source)

```bash
# Frontend deps
cd frontend && npm install && cd ..

# Dev (hot reload)
wails3 dev
# or
task dev
```

Production packages:

```bash
wails3 task linux:create:deb
wails3 task windows:create:msi ARCH=amd64   # needs wixl on Linux
```

Regenerate TypeScript bindings after changing exported Go methods:

```bash
wails3 generate bindings -ts -d frontend/bindings ./...
```

## CLI

```bash
go build -o bin/sendsmtp-cli ./cmd/sendsmtp

cp data/smtps.example.txt data/smtps.txt
cp data/emails.example.txt data/emails.txt
cp data/assuntos.example.txt data/assuntos.txt
cp data/links.example.txt data/links.txt
cp data/msg.example.html data/msg.html

./bin/sendsmtp-cli import all
./bin/sendsmtp-cli status
./bin/sendsmtp-cli send
./bin/sendsmtp-cli import emails --validate
```

## Configuration

Runtime settings live in [`app.config.yml`](app.config.yml) (workers, timeouts, paths).  
Do not confuse with [`build/config.yml`](build/config.yml) (Wails packaging).

SQLite DB path defaults to `data/sendsmtp.db` (gitignored).

## SMTP import

### `email;password` — auto discovery

```
user@company.com;SecretPass
billing@example.com.br;AnotherPass
```

Flow:

1. Parse domain from the address
2. Look up **MX** and map to a submission host when possible (e.g. Locaweb → `email-ssl.com.br`)
3. Probe AUTH on 587 STARTTLS and 465 SSL
4. Persist only accounts that authenticate

Known consumer domains (Gmail, Outlook/M365, Yahoo, iCloud, Zoho, Proton, Locaweb, …) use fixed submission hosts. Custom domains use MX fingerprints. Inbound relays (`mx.*`) are **not** used as submission targets.

### Goscan — explicit host

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

`from` / `user` must be real emails. Values like `${MAIL_USERNAME}` are **not** expanded; the engine sanitizes them and may fall back to `user` when it is a valid address.

## IMAP contact extract

On the **SMTPs** page → Extract contacts (one or all):

1. Discover IMAP (SMTP host hint + `imap.` / `mail.` / provider hosts such as `email-ssl.com.br`)
2. Scan INBOX + Sent (~150 messages)
3. Collect contact addresses and `email;password` pairs from headers/bodies
4. Write files under `data/extracted/`
5. **Import contacts into the Emails queue automatically** (reports inserted vs already present)
6. Import credentials as SMTPs (with discovery)

## Emails

- Import always normalizes and deduplicates (`UNIQUE(address)` — no re-send to addresses already stored)
- Large lists: use **Import file** (path-only IPC). Huge pastes can break the Wails runtime (~512KB chunk mismatch)
- **Validate**: syntax + resolvable MX + disposable blocklist; invalid/duplicate lines are skipped. DNS lookups are paced
- Status counts are cached in `email_counts` for a fast dashboard on large DBs
- UI: pagination, search, status filters

## Templates

Working files: `data/msg.html`, `data/assuntos.txt` (subjects). See [`data/README.md`](data/README.md).

| Placeholder | Meaning |
|-------------|---------|
| `{{email}}` | Recipient |
| `{{link}}` | Link from the list + `?p=<email>` at send time |
| `{{assunto}}` / `{{subject}}` | Processed subject line |
| `{{from}}` | SMTP From (sanitized) |
| `{{uniq}}` / `{{id}}` | Per-send unique id |

Do not hardcode `?p=` in HTML or `links.txt` — `mailer.PersonalizeLink` adds it.

Spintax: `{a|b|c}` (pipe required). `{Status}` alone stays literal.

Optional footer From:

```html
<span data-from>{{from}}</span>
```

## Inbox check (MailReach)

**Check / Analyze** uses the [MailReach free spam test](https://www.mailreach.co/email-spam-test). Free tier ≈ 3 tests / 24h.

```bash
cd scripts/inbox-check && npm i && npx playwright install chromium
```

## Project layout

```
app.go / main.go          Wails service + entrypoint
cmd/sendsmtp/             Headless CLI
internal/
  engine/                 Shared orchestration
  store/                  SQLite
  smtpdiscover/           MX → SMTP + AUTH
  imapdiscover/           IMAP discovery
  imapextract/            Contacts / passwords
  mailer/                 HTML, spintax, personalized links
  emailvalid/             Syntax + MX validation
  worker/                 Campaign workers
frontend/                 React UI + generated bindings
data/                     Local content (secrets gitignored)
build/                    Wails packaging
```

## Security

**Never commit:**

- `data/smtps.txt`, `data/emails.txt`
- `data/sendsmtp.db*`
- `data/extracted/` (contacts and passwords)
- Real credentials anywhere in the tree

Use `data/*.example.*` for setup. See [`.gitignore`](.gitignore).

## Cursor agent commands

In Agent chat, type `/` and pick: `dev`, `build`, `import-all`, `send`, `status`, `test-smtp`, `reset-failed`, `add-smtp-help`.

## i18n

UI languages: **English** (default for non-`pt*` browsers) and **Português**. Switch under **Settings → Language** (stored in `localStorage`).

All React copy goes through `useTranslation()` / `t("key")` — catalogs in `frontend/src/i18n/locales/`. Agent skill: `.cursor/skills/sendsmtp-i18n`.

## License

MIT — see [LICENSE](LICENSE).

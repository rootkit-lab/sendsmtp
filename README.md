# SendSMTP

App desktop para campanhas SMTP: **Wails v3** + **React/TypeScript** + Tailwind + motor Go + **SQLite**.

UI e CLI compartilham o mesmo engine (`internal/`).

## Requisitos

- Go 1.25+
- Node.js 20+ (frontend)
- [Wails v3](https://v3.wails.io/) CLI (`wails3`)
- Linux (WebKitGTK) / demais plataformas conforme Wails

## Setup

```bash
go version
cd frontend && npm install && cd ..

# Dev (hot reload)
wails3 dev
# ou
task dev
```

Build nativo: `task build` (ou o Taskfile da pasta `build/`).

Bindings TypeScript (após mudar `app.go` / métodos públicos):

```bash
wails3 generate bindings -ts -d frontend/bindings ./...
```

## CLI headless

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

## Config

Paths, workers e timeouts: `app.config.yml` (não confundir com `build/config.yml` do packaging Wails).

DB SQLite: `data/sendsmtp.db` (gitignored).

## SMTP

### `email;senha` — discovery automático

```
alberto.santos@empresa.com.br;SenhaAqui
atendimento@creluz.com.br;@Creluz2026
```

Fluxo:

1. Lê domínio do e-mail
2. Consulta **MX** e mapeia provedor (ex.: Locaweb → `email-ssl.com.br`)
3. Testa AUTH em 587 STARTTLS / 465 SSL
4. Salva só o que autenticar

Provedores conhecidos por domínio de e-mail: Gmail, Outlook/M365, Yahoo, iCloud, Zoho, Proton, Locaweb, etc. Domínios customizados usam fingerprint do MX (Locaweb, Google Workspace, Microsoft 365, KingHost, …).

Hosts MX de **entrada** (`mx.*`) não são usados como SMTP de envio.

### Goscan (host explícito)

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

`from` / `user` devem ser e-mails reais. `${MAIL_USERNAME}` **não** é expandido — o motor sanitiza e tenta cair no `user` quando for e-mail válido.

## Extrair contatos (IMAP)

Na página **SMTPs** → Extrair contatos (um ou todos):

1. Descobre IMAP (hint do host SMTP + `imap.` / `mail.` / Locaweb `email-ssl`)
2. Lê INBOX + Sent (~150 msgs)
3. Extrai contatos e pares `email;senha`
4. Grava em `data/extracted/`
5. **Importa contatos automaticamente na lista de Emails** (novos + já existentes reportados)
6. Importa credenciais como SMTPs (com discovery)

## Emails

- Import **sempre** normaliza e deduplica (`UNIQUE(address)` — não reenvia o que já está no DB).
- Listas grandes (&gt;~10k / paste &gt;~400KB): use **Importar arquivo** (só o caminho passa pelo IPC; colar grande quebra o runtime Wails).
- **Validar**: sintaxe + **MX real** por domínio + bloqueio de descartáveis; inválidos/duplicados não entram. DNS é paced (não flood).
- Contadores (`pending` / `sent` / …) em cache (`email_counts`) — dashboard leve mesmo com centenas de milhares de linhas.
- Listagem: paginação, busca e filtros por status.

## Templates

Arquivos: `data/msg.html`, `data/assuntos.txt` (ver `data/README.md`). Tema exemplo: nota fiscal / NF-e.

| Tag | Significado |
|-----|-------------|
| `{{email}}` | Destinatário |
| `{{link}}` | Link da lista + `?p=<email>` no envio (auto) |
| `{{assunto}}` / `{{subject}}` | Assunto processado |
| `{{from}}` | From do SMTP (sanitizado) |
| `{{uniq}}` / `{{id}}` | ID único por envio |

Não coloque `?p=` no HTML ou em `links.txt` — `mailer.PersonalizeLink` faz isso.

Spintax: `{a|b|c}` (precisa `|`). `{Status}` sozinho fica literal.

Rodapé opcional:

```html
<span data-from>{{from}}</span>
```

## Inbox check (MailReach)

**Check / Analise** (um ou todos) usa o [spam test free do MailReach](https://www.mailreach.co/email-spam-test). Limite free ≈ 3 testes / 24h.

```bash
cd scripts/inbox-check && npm i && npx playwright install chromium
```

## Estrutura

```
app.go / main.go     # Wails service + entry
cmd/sendsmtp/        # CLI
internal/
  engine/            # orquestra UI/CLI
  store/             # SQLite
  smtpdiscover/      # MX → SMTP + AUTH
  imapdiscover/      # IMAP
  imapextract/       # contatos / senhas
  mailer/            # HTML, spintax, link ?p=
  emailvalid/        # sintaxe + MX
  worker/            # campanha
frontend/            # React UI
data/                # conteúdo local (secrets gitignored)
```

## Segurança / Git

**Não commitar:**

- `data/smtps.txt`, `data/emails.txt`, `data/sendsmtp.db*`
- `data/extracted/` (contatos e senhas)
- senhas em qualquer arquivo

Use os `*.example.*` no setup.

## Cursor

No Agent chat: `/dev` `/build` `/import-all` `/send` `/status` `/test-smtp` `/reset-failed` `/add-smtp-help`

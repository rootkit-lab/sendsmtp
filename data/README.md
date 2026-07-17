# data/

Arquivos de conteúdo e exemplos. Paths reais vêm de `app.config.yml`.

| Arquivo | Uso |
|---------|-----|
| `msg.html` | Corpo HTML da campanha (nota fiscal) |
| `msg.example.html` | Mesmo template (para `cp` no setup) |
| `assuntos.txt` | Assuntos NF-e (1/linha), spintax + `{{uniq}}` |
| `links.txt` | URLs (1/linha) |
| `emails.txt` | Destinatários (1/linha) — listas grandes: UI **Importar arquivo** |
| `smtps.txt` | Blocos goscan **ou** `email;senha` (**não commitar**) |
| `sendsmtp.db` | SQLite (**não commitar**) |
| `extracted/` | Contatos/senhas do IMAP (**não commitar**) |

Validação no import (opcional): sintaxe + MX DNS + bloqueio de descartáveis; inválidos/duplicados não entram.

Extrair contatos (SMTPs): grava aqui **e** importa automaticamente na lista de Emails.

## Placeholders

`{{email}}` `{{link}}` `{{assunto}}` `{{subject}}` `{{from}}` `{{uniq}}` `{{id}}`

No envio, `{{link}}` recebe automaticamente `?p=<destinatário>`:

`https://exemplo.com/` → `https://exemplo.com/?p=user%40gmail.com`

Não adicione `?p=` no HTML ou em `links.txt` — o motor (`mailer.PersonalizeLink`) faz isso.

## Spintax

`{a|b|c}` — obrigatório `|`. `{Status}` sozinho **não** funciona.

## From no rodapé

```html
{Notificação automática|Comprovante automático}<span data-from>{{from}}</span> · {{uniq}}
```

Se o From do SMTP for `${MAIL_USERNAME}` ou vazio, o trecho some. Corrija o campo `from:` no goscan para um e-mail real.

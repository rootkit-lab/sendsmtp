# Contributing

## Development

```bash
cd frontend && npm install && cd ..
wails3 dev   # or: task dev
```

After changing exported methods on `AppService` / `app.go`:

```bash
wails3 generate bindings -ts -d frontend/bindings ./...
```

Run Go tests:

```bash
go test ./internal/...
```

## Guidelines

- Keep secrets out of git (`data/smtps.txt`, DB, `data/extracted/`).
- Prefer small, focused commits.
- Match existing Go/React style; avoid drive-by refactors.
- Docs and user-facing project markdown are in **English**.
- UI strings must use i18n (`t("key")`); add keys to both `en.ts` and `pt.ts`. See `.cursor/skills/sendsmtp-i18n`.

## Pull requests

Describe what changed and how to verify (import SMTP, send a small list, extract contacts, etc.).

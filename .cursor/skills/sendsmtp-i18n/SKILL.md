---
name: sendsmtp-i18n
description: >-
  Requires SendSMTP UI copy to go through i18n (useTranslation / t()). Use whenever
  adding or editing frontend React text, labels, toasts, confirms, placeholders,
  or Settings language support in this project.
---

# SendSMTP i18n

All user-visible frontend strings must use the project i18n layer. Do not hardcode English or Portuguese in JSX.

## Setup

- Provider: `I18nProvider` in `frontend/src/main.tsx`
- Hook: `import { useTranslation } from "@/i18n"` → `const { t, locale, setLocale } = useTranslation()`
- Catalogs: `frontend/src/i18n/locales/en.ts` (source of truth for keys) and `pt.ts`
- Language switch: Settings → Language (`en` | `pt`), persisted in `localStorage` key `sendsmtp.locale`

## Rules

1. **Never** put user-facing string literals in components (`"Import"`, `"Carregando…"`, toast messages, `window.confirm`, dialog titles, placeholders, empty states).
2. Add the key to **`en.ts` first**, then mirror it in **`pt.ts`** with the same `MessageKey`.
3. Use `t("key")` or `t("key", { name: value })` for `{{name}}` interpolation.
4. Avoid `{{...}}` in locale strings when the text is documenting template placeholders (e.g. email HTML tags). Prefer plain words (`email, link, assunto`) so interpolate does not strip them.
5. Status enums from the backend (`pending`, `sent`, `failed`, hostnames, error payloads) can stay as raw data; wrap UI chrome labels only.
6. When adding a new page/section, group keys: `nav.*`, `dash.*`, `emails.*`, `smtps.*`, `content.*`, `settings.*`, `common.*`.

## Example

```tsx
import { useTranslation } from "@/i18n";

export function Example() {
  const { t } = useTranslation();
  return (
    <button onClick={() => toast.success(t("emails.toast.requeued", { n: 3 }))}>
      {t("emails.resetFailed")}
    </button>
  );
}
```

## Checklist before finishing UI work

- [ ] No new hardcoded UI copy in `frontend/src/**/*.tsx`
- [ ] New keys in both `en.ts` and `pt.ts`
- [ ] `npx tsc --noEmit` in `frontend/` passes

export type Locale = "en" | "pt";

export const LOCALES: { id: Locale; label: string }[] = [
  { id: "en", label: "English" },
  { id: "pt", label: "Português" },
];

export const STORAGE_KEY = "sendsmtp.locale";

export type Params = Record<string, string | number | boolean | null | undefined>;

export function interpolate(template: string, params?: Params): string {
  if (!params) return template;
  return template.replace(/\{\{(\w+)\}\}/g, (_, key: string) => {
    const v = params[key];
    return v == null ? "" : String(v);
  });
}

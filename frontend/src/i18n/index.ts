import { createContext, createElement, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { en, type MessageKey } from "./locales/en";
import { pt } from "./locales/pt";
import { interpolate, LOCALES, STORAGE_KEY, type Locale, type Params } from "./types";

const catalogs: Record<Locale, Record<MessageKey, string>> = {
  en: en as Record<MessageKey, string>,
  pt,
};

function detectLocale(): Locale {
  try {
    const saved = localStorage.getItem(STORAGE_KEY) as Locale | null;
    if (saved === "en" || saved === "pt") return saved;
  } catch {
    /* ignore */
  }
  const nav = typeof navigator !== "undefined" ? navigator.language.toLowerCase() : "en";
  return nav.startsWith("pt") ? "pt" : "en";
}

type I18nValue = {
  locale: Locale;
  setLocale: (l: Locale) => void;
  t: (key: MessageKey, params?: Params) => string;
  locales: typeof LOCALES;
};

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const initial = detectLocale();
    if (typeof document !== "undefined") {
      document.documentElement.lang = initial === "pt" ? "pt-BR" : "en";
    }
    return initial;
  });

  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l);
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      /* ignore */
    }
    if (typeof document !== "undefined") {
      document.documentElement.lang = l === "pt" ? "pt-BR" : "en";
    }
  }, []);

  const t = useCallback(
    (key: MessageKey, params?: Params) => {
      const dict = catalogs[locale] || catalogs.en;
      const raw = dict[key] ?? catalogs.en[key] ?? key;
      return interpolate(raw, params);
    },
    [locale]
  );

  const value = useMemo(() => ({ locale, setLocale, t, locales: LOCALES }), [locale, setLocale, t]);

  return createElement(I18nContext.Provider, { value }, children);
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}

export function useTranslation() {
  const { t, locale, setLocale } = useI18n();
  return { t, locale, setLocale };
}

export type { MessageKey, Locale, Params };
export { LOCALES };

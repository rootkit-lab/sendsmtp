import { useEffect, useState } from "react";
import { GetConfig, ImportAll, SaveConfig } from "../../bindings/github.com/wiz/sendsmtp/appservice";
import type { Config } from "../../bindings/github.com/wiz/sendsmtp/internal/config/models";
import { Button } from "@/components/ui/button";
import { Input, Label } from "@/components/ui/form";
import { useI18n, type Locale } from "@/i18n";
import { toast } from "sonner";

export function SettingsPage() {
  const { t, locale, setLocale, locales } = useI18n();
  const [cfg, setCfg] = useState<Config | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    GetConfig()
      .then((c) => setCfg(c))
      .catch((e) => toast.error(String(e?.message ?? e)));
  }, []);

  if (!cfg) {
    return <p className="text-stone-500">{t("common.loading")}</p>;
  }

  function num(key: keyof Config, value: string) {
    setCfg({ ...cfg!, [key]: Number(value) || 0 } as Config);
  }

  function inboxNum(key: "wait_sec" | "timeout_sec", value: string) {
    setCfg({
      ...cfg!,
      inbox_check: {
        ...(cfg!.inbox_check || { headless: true, wait_sec: 60, timeout_sec: 240, seeds: [] }),
        [key]: Number(value) || 0,
      },
    });
  }

  return (
    <div className="mx-auto max-w-3xl space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-display)] text-3xl">{t("settings.title")}</h1>
        <p className="mt-1 text-stone-500">{t("settings.subtitle")}</p>
      </header>

      <section className="space-y-3 rounded-lg border border-stone-300/80 bg-white/60 p-4">
        <h2 className="text-sm font-semibold text-stone-800">{t("settings.language")}</h2>
        <p className="text-xs text-stone-500">{t("settings.languageHint")}</p>
        <div className="flex flex-wrap gap-2">
          {locales.map((l) => (
            <Button
              key={l.id}
              size="sm"
              variant={locale === l.id ? "default" : "outline"}
              onClick={() => setLocale(l.id as Locale)}
            >
              {l.label}
            </Button>
          ))}
        </div>
      </section>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("settings.workers")} value={cfg.workers} onChange={(v) => num("workers", v)} />
        <Field label={t("settings.smtpMaxConn")} value={cfg.smtp_max_conn} onChange={(v) => num("smtp_max_conn", v)} />
        <Field label={t("settings.dialTimeout")} value={cfg.dial_timeout_sec} onChange={(v) => num("dial_timeout_sec", v)} />
        <Field label={t("settings.sendTimeout")} value={cfg.send_timeout_sec} onChange={(v) => num("send_timeout_sec", v)} />
        <Field label={t("settings.retryMax")} value={cfg.retry_max} onChange={(v) => num("retry_max", v)} />
        <Field
          label={t("settings.disableAfter")}
          value={cfg.smtp_disable_after_fails}
          onChange={(v) => num("smtp_disable_after_fails", v)}
        />
        <div className="sm:col-span-2">
          <Label>{t("settings.fromName")}</Label>
          <Input value={cfg.from_name || ""} onChange={(e) => setCfg({ ...cfg, from_name: e.target.value })} />
        </div>
        <div className="sm:col-span-2">
          <Label>{t("settings.database")}</Label>
          <Input value={cfg.database} onChange={(e) => setCfg({ ...cfg, database: e.target.value })} />
        </div>
      </div>

      <section className="space-y-3 rounded-lg border border-stone-300/80 bg-white/60 p-4">
        <h2 className="text-sm font-semibold text-stone-800">{t("settings.inboxTitle")}</h2>
        <p className="text-xs text-stone-500">
          {t("settings.inboxBody")}{" "}
          <a
            className="underline"
            href="https://www.mailreach.co/email-spam-test"
            target="_blank"
            rel="noreferrer"
          >
            {t("settings.inboxLink")}
          </a>
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field
            label={t("settings.waitAfter")}
            value={cfg.inbox_check?.wait_sec || 60}
            onChange={(v) => inboxNum("wait_sec", v)}
          />
          <Field
            label={t("settings.pollTimeout")}
            value={cfg.inbox_check?.timeout_sec || 240}
            onChange={(v) => inboxNum("timeout_sec", v)}
          />
        </div>
        <label className="flex items-center gap-2 text-sm text-stone-700">
          <input
            type="checkbox"
            checked={cfg.inbox_check?.headless !== false}
            onChange={(e) =>
              setCfg({
                ...cfg,
                inbox_check: {
                  ...(cfg.inbox_check || { wait_sec: 60, timeout_sec: 240, seeds: [] }),
                  headless: e.target.checked,
                },
              })
            }
          />
          {t("settings.headless")}
        </label>
      </section>

      <section className="space-y-3 rounded-lg border border-stone-300/80 bg-white/60 p-4">
        <h2 className="text-sm font-semibold text-stone-800">{t("settings.shortenerTitle")}</h2>
        <p className="text-xs text-stone-500">{t("settings.shortenerBody")}</p>
        <label className="flex items-center gap-2 text-sm text-stone-700">
          <input
            type="checkbox"
            checked={!!cfg.shortener?.enabled}
            onChange={(e) =>
              setCfg({
                ...cfg,
                shortener: {
                  ...(cfg.shortener || { every_n: 100, batch_size: 10, concurrency: 6 }),
                  enabled: e.target.checked,
                },
              })
            }
          />
          {t("settings.shortenerEnabled")}
        </label>
        <div className="grid gap-3 sm:grid-cols-3">
          <div>
            <Label>{t("settings.shortenerEveryN")}</Label>
            <Input
              type="number"
              value={cfg.shortener?.every_n ?? 100}
              onChange={(e) =>
                setCfg({
                  ...cfg,
                  shortener: {
                    ...(cfg.shortener || { enabled: false, batch_size: 10, concurrency: 6 }),
                    every_n: Number(e.target.value) || 0,
                  },
                })
              }
            />
          </div>
          <div>
            <Label>{t("settings.shortenerBatch")}</Label>
            <Input
              type="number"
              value={cfg.shortener?.batch_size ?? 10}
              onChange={(e) =>
                setCfg({
                  ...cfg,
                  shortener: {
                    ...(cfg.shortener || { enabled: false, every_n: 100, concurrency: 6 }),
                    batch_size: Number(e.target.value) || 0,
                  },
                })
              }
            />
          </div>
          <div>
            <Label>{t("settings.shortenerConc")}</Label>
            <Input
              type="number"
              value={cfg.shortener?.concurrency ?? 6}
              onChange={(e) =>
                setCfg({
                  ...cfg,
                  shortener: {
                    ...(cfg.shortener || { enabled: false, every_n: 100, batch_size: 10 }),
                    concurrency: Number(e.target.value) || 0,
                  },
                })
              }
            />
          </div>
        </div>
      </section>

      <div className="grid gap-3 sm:grid-cols-2">
        {(["smtps", "emails", "subjects", "links", "html"] as const).map((k) => (
          <div key={k}>
            <Label>{t("settings.path", { key: k })}</Label>
            <Input
              value={(cfg.paths as any)?.[k] || ""}
              onChange={(e) =>
                setCfg({
                  ...cfg,
                  paths: { ...cfg.paths, [k]: e.target.value },
                })
              }
            />
          </div>
        ))}
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            try {
              await SaveConfig(cfg);
              toast.success(t("settings.saved"));
            } catch (e: any) {
              toast.error(String(e?.message ?? e));
            } finally {
              setBusy(false);
            }
          }}
        >
          {t("settings.save")}
        </Button>
        <Button
          variant="secondary"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            try {
              const res = await ImportAll();
              const keys = Object.keys(res || {}).join(", ") || t("settings.importAllEmpty");
              toast.success(t("settings.importAllResult", { keys }));
            } catch (e: any) {
              toast.error(String(e?.message ?? e));
            } finally {
              setBusy(false);
            }
          }}
        >
          {t("settings.importAll")}
        </Button>
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label>{label}</Label>
      <Input type="number" value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

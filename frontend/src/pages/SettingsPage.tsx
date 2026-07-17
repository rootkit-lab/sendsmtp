import { useEffect, useState } from "react";
import { GetConfig, ImportAll, SaveConfig } from "../../bindings/github.com/wiz/sendsmtp/appservice";
import type { Config } from "../../bindings/github.com/wiz/sendsmtp/internal/config/models";
import { Button } from "@/components/ui/button";
import { Input, Label } from "@/components/ui/form";
import { toast } from "sonner";

export function SettingsPage() {
  const [cfg, setCfg] = useState<Config | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    GetConfig()
      .then((c) => setCfg(c))
      .catch((e) => toast.error(String(e?.message ?? e)));
  }, []);

  if (!cfg) {
    return <p className="text-stone-500">Carregando…</p>;
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
        <h1 className="font-[family-name:var(--font-display)] text-3xl">Settings</h1>
        <p className="mt-1 text-stone-500">Workers, paths e spam test MailReach</p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Workers" value={cfg.workers} onChange={(v) => num("workers", v)} />
        <Field label="SMTP max conn" value={cfg.smtp_max_conn} onChange={(v) => num("smtp_max_conn", v)} />
        <Field label="Dial timeout (s)" value={cfg.dial_timeout_sec} onChange={(v) => num("dial_timeout_sec", v)} />
        <Field label="Send timeout (s)" value={cfg.send_timeout_sec} onChange={(v) => num("send_timeout_sec", v)} />
        <Field label="Retry max" value={cfg.retry_max} onChange={(v) => num("retry_max", v)} />
        <Field
          label="Disable SMTP after fails"
          value={cfg.smtp_disable_after_fails}
          onChange={(v) => num("smtp_disable_after_fails", v)}
        />
        <div className="sm:col-span-2">
          <Label>From name</Label>
          <Input value={cfg.from_name || ""} onChange={(e) => setCfg({ ...cfg, from_name: e.target.value })} />
        </div>
        <div className="sm:col-span-2">
          <Label>Database</Label>
          <Input value={cfg.database} onChange={(e) => setCfg({ ...cfg, database: e.target.value })} />
        </div>
      </div>

      <section className="space-y-3 rounded-lg border border-stone-300/80 bg-white/60 p-4">
        <h2 className="text-sm font-semibold text-stone-800">Inbox check (MailReach free)</h2>
        <p className="text-xs text-stone-500">
          Usa o{" "}
          <a
            className="underline"
            href="https://www.mailreach.co/email-spam-test"
            target="_blank"
            rel="noreferrer"
          >
            spam test gratuito do MailReach
          </a>
          : cria o teste, envia o HTML da campanha (+ código) para ~28 seeds deles e faz poll do score.
          Limite free ≈ 3 testes / 24h. Playwright só entra se a API HTTP for bloqueada — setup:{" "}
          <code className="rounded bg-stone-100 px-1">
            cd scripts/inbox-check && npm i && npx playwright install chromium
          </code>
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field
            label="Wait after send (s)"
            value={cfg.inbox_check?.wait_sec || 60}
            onChange={(v) => inboxNum("wait_sec", v)}
          />
          <Field
            label="Poll timeout (s)"
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
          Headless browser (fallback Playwright)
        </label>
      </section>

      <div className="grid gap-3 sm:grid-cols-2">
        {(["smtps", "emails", "subjects", "links", "html"] as const).map((k) => (
          <div key={k}>
            <Label>Path {k}</Label>
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
              toast.success("Config salva");
            } catch (e: any) {
              toast.error(String(e?.message ?? e));
            } finally {
              setBusy(false);
            }
          }}
        >
          Salvar config
        </Button>
        <Button
          variant="secondary"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            try {
              const res = await ImportAll();
              toast.success(`Import all: ${Object.keys(res || {}).join(", ") || "nada"}`);
            } catch (e: any) {
              toast.error(String(e?.message ?? e));
            } finally {
              setBusy(false);
            }
          }}
        >
          Import all (arquivos)
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

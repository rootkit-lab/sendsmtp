import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  AnalyzeAllSmtps,
  AnalyzeSmtp,
  ExtractAllSmtpContacts,
  ExtractSmtpContacts,
  ImportSmtpsText,
  ListSmtps,
  SetSmtpActive,
  TestSmtp,
} from "../../bindings/github.com/wiz/sendsmtp/appservice";
import type { ExtractMailboxResult } from "../../bindings/github.com/wiz/sendsmtp/internal/engine/models";
import type { SMTP } from "../../bindings/github.com/wiz/sendsmtp/internal/store/models";
import { Button } from "@/components/ui/button";
import { Badge, Label, Textarea } from "@/components/ui/form";
import { ProgressBar, eventData, type ProgressInfo } from "@/components/ui/progress";
import { useTranslation } from "@/i18n";
import { toast } from "sonner";

function inboxTone(label: string): "ok" | "warn" | "danger" | "neutral" | "accent" {
  switch (label) {
    case "inbox":
      return "ok";
    case "mixed":
      return "warn";
    case "spam":
      return "danger";
    default:
      return "neutral";
  }
}

export function SmtpsPage() {
  const { t } = useTranslation();
  const [raw, setRaw] = useState("");
  const [list, setList] = useState<SMTP[]>([]);
  const [busy, setBusy] = useState(false);
  const [analyzing, setAnalyzing] = useState<number | null>(null);
  const [analyzingAll, setAnalyzingAll] = useState(false);
  const [extracting, setExtracting] = useState<number | null>(null);
  const [extractingAll, setExtractingAll] = useState(false);
  const [progress, setProgress] = useState<ProgressInfo | null>(null);
  const [lastExtract, setLastExtract] = useState<ExtractMailboxResult | null>(null);

  const refresh = useCallback(async () => {
    try {
      setList((await ListSmtps()) || []);
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    const off = Events.On("job:progress", (ev: any) => {
      const data = eventData<ProgressInfo>(ev);
      if (!data) return;
      if (
        data.job !== "smtps-import" &&
        data.job !== "analyze" &&
        data.job !== "analyze-all" &&
        data.job !== "extract-contacts"
      ) {
        return;
      }
      setProgress(data.done ? null : data);
    });
    return () => {
      off?.();
    };
  }, []);

  const working = busy || analyzing !== null || analyzingAll || extracting !== null || extractingAll;

  async function onImport() {
    setBusy(true);
    setProgress({ job: "smtps-import", phase: "parse", percent: 2, message: t("smtps.toast.importing") });
    try {
      const res = await ImportSmtpsText(raw);
      toast.success(
        t("smtps.toast.imported", {
          inserted: res.inserted,
          updated: res.updated,
          invalid: res.invalid ? t("smtps.toast.invalidPart", { n: res.invalid }) : "",
        })
      );
      setRaw("");
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setBusy(false);
      setProgress(null);
    }
  }

  async function onAnalyze(id: number) {
    setAnalyzing(id);
    setProgress({
      job: "analyze",
      phase: "create",
      smtp_id: id,
      percent: 2,
      message: t("smtps.progress.checkOne", { id }),
    });
    try {
      const sum = await AnalyzeSmtp(id);
      toast.success(
        t("smtps.toast.analyze", {
          id,
          label: sum.label,
          score: Number(sum.score).toFixed(0),
          inbox: sum.inbox,
          total: sum.total,
          url: sum.report_url ? ` · ${sum.report_url}` : "",
        })
      );
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setAnalyzing(null);
      setProgress(null);
    }
  }

  async function onExtract(id: number) {
    setExtracting(id);
    setLastExtract(null);
    setProgress({
      job: "extract-contacts",
      phase: "start",
      smtp_id: id,
      percent: 2,
      message: t("smtps.progress.extractOne", { id }),
    });
    try {
      const res = await ExtractSmtpContacts(id);
      setLastExtract(res);
      const n = Number(res.contact_count) || res.contacts?.length || 0;
      toast.success(
        t("smtps.toast.extract", {
          id,
          n,
          imported: res.imported_emails,
          skipped: res.skipped_emails,
          creds: res.credentials?.length || 0,
          imap: res.imap_host ? ` · ${res.imap_host}:${res.imap_port}` : "",
        })
      );
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setExtracting(null);
      setProgress(null);
    }
  }

  async function onAnalyzeAll() {
    if (!list.length) return;
    if (!window.confirm(t("smtps.confirmAnalyzeAll", { n: list.length }))) {
      return;
    }
    setAnalyzingAll(true);
    setProgress({
      job: "analyze-all",
      phase: "start",
      percent: 1,
      current: 0,
      total: list.length,
      message: t("smtps.progress.checkAll", { n: list.length }),
    });
    try {
      const res = await AnalyzeAllSmtps();
      toast.success(
        t("smtps.toast.analyzeAll", {
          ok: res.ok,
          failed: res.failed,
          batches: res.batches,
          total: res.total,
        })
      );
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setAnalyzingAll(false);
      setProgress(null);
    }
  }

  async function onExtractAll() {
    if (!list.length) return;
    if (!window.confirm(t("smtps.confirmExtractAll", { n: list.length }))) {
      return;
    }
    setExtractingAll(true);
    setLastExtract(null);
    setProgress({
      job: "extract-contacts",
      phase: "batch",
      percent: 1,
      current: 0,
      total: list.length,
      message: t("smtps.progress.extractAll", { n: list.length }),
    });
    try {
      const map = await ExtractAllSmtpContacts();
      let contacts = 0;
      let creds = 0;
      let imported = 0;
      let skipped = 0;
      let ok = 0;
      Object.values(map || {}).forEach((r) => {
        if (r?.imap_host) ok++;
        contacts += Number(r?.contact_count) || r?.contacts?.length || 0;
        creds += r?.credentials?.length || 0;
        imported += Number(r?.imported_emails) || 0;
        skipped += Number(r?.skipped_emails) || 0;
      });
      toast.success(
        t("smtps.toast.extractAll", {
          ok,
          contacts,
          imported,
          skipped,
          creds,
        })
      );
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setExtractingAll(false);
      setProgress(null);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-display)] text-3xl">{t("smtps.title")}</h1>
        <p className="mt-1 text-stone-500">{t("smtps.subtitle")}</p>
      </header>

      <section className="space-y-3">
        <Label>{t("smtps.pasteLabel")}</Label>
        <p className="text-xs text-stone-500">{t("smtps.pasteHint")}</p>
        <Textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          placeholder={t("smtps.placeholder")}
          className="min-h-[200px]"
          disabled={working}
        />
        <ProgressBar info={working ? progress : null} />
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || !raw.trim() || working} onClick={onImport}>
            {busy ? t("smtps.importing") : t("smtps.import")}
          </Button>
          <Button variant="secondary" disabled={working || !list.length} onClick={onAnalyzeAll}>
            {analyzingAll ? t("smtps.checkAllBusy") : t("smtps.checkAll")}
          </Button>
          <Button variant="secondary" disabled={working || !list.length} onClick={onExtractAll}>
            {extractingAll ? t("smtps.extractAllBusy") : t("smtps.extractAll")}
          </Button>
        </div>
      </section>

      {lastExtract ? (
        <section className="rounded-lg border border-stone-300/80 bg-white/70 p-4 text-sm">
          <h2 className="font-medium text-stone-800">{t("smtps.lastExtract")}</h2>
          <p className="mt-1 text-stone-600">
            {t("smtps.lastExtractMeta", {
              host: lastExtract.imap_host,
              port: lastExtract.imap_port,
              msgs: lastExtract.messages_scanned,
              contacts: Number(lastExtract.contact_count) || lastExtract.contacts?.length || 0,
              creds: lastExtract.credentials?.length || 0,
            })}
          </p>
          <p className="mt-1 text-teal-800">
            {t("smtps.lastExtractQueue", {
              imported: lastExtract.imported_emails,
              skipped: lastExtract.skipped_emails
                ? t("smtps.lastExtractSkipped", { n: lastExtract.skipped_emails })
                : "",
              smtps:
                lastExtract.imported_smtps || lastExtract.updated_smtps
                  ? t("smtps.lastExtractSmtps", {
                      ins: lastExtract.imported_smtps,
                      upd: lastExtract.updated_smtps,
                    })
                  : "",
            })}
          </p>
          <p className="mt-1 font-mono text-xs text-stone-500">
            {lastExtract.contacts_file}
            {lastExtract.creds_file ? ` · ${lastExtract.creds_file}` : ""}
          </p>
          {lastExtract.contacts?.length ? (
            <details className="mt-2">
              <summary className="cursor-pointer text-xs text-stone-500">{t("smtps.viewContacts")}</summary>
              <pre className="mt-1 max-h-40 overflow-auto rounded bg-stone-50 p-2 text-xs">
                {(lastExtract.contacts || []).slice(0, 200).join("\n")}
              </pre>
            </details>
          ) : null}
          {lastExtract.credentials?.length ? (
            <details className="mt-2">
              <summary className="cursor-pointer text-xs text-stone-500">{t("smtps.viewCreds")}</summary>
              <pre className="mt-1 max-h-40 overflow-auto rounded bg-stone-50 p-2 text-xs">
                {(lastExtract.credentials || [])
                  .slice(0, 100)
                  .map((c) => `${c.email};${c.password}`)
                  .join("\n")}
              </pre>
            </details>
          ) : null}
        </section>
      ) : null}

      <section className="overflow-x-auto overflow-hidden rounded-lg border border-stone-300/80 bg-white/70">
        <table className="w-full min-w-[900px] text-left text-sm">
          <thead className="border-b border-stone-300 bg-stone-100/80 text-xs uppercase text-stone-500">
            <tr>
              <th className="px-3 py-2">{t("common.host")}</th>
              <th className="px-3 py-2">{t("common.from")}</th>
              <th className="px-3 py-2">{t("common.status")}</th>
              <th className="px-3 py-2">{t("common.inbox")}</th>
              <th className="px-3 py-2">{t("common.score")}</th>
              <th className="px-3 py-2">{t("common.sent")}</th>
              <th className="px-3 py-2">{t("common.actions")}</th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => (
              <tr key={s.id} className="border-b border-stone-200/80 align-top">
                <td className="px-3 py-2 font-mono text-xs">
                  {s.host}:{s.port}
                </td>
                <td className="px-3 py-2">{s.from_addr}</td>
                <td className="px-3 py-2">
                  <Badge tone={s.status === "active" ? "ok" : "danger"}>
                    {s.status === "active" ? t("common.active") : t("common.disabled")}
                  </Badge>
                </td>
                <td className="px-3 py-2">
                  {s.inbox_label ? (
                    <div className="space-y-1">
                      <Badge tone={inboxTone(s.inbox_label)}>{s.inbox_label}</Badge>
                      {s.inbox_rate >= 0 ? (
                        <div className="text-xs text-stone-500">{Number(s.inbox_rate).toFixed(0)}% inbox</div>
                      ) : null}
                    </div>
                  ) : (
                    <span className="text-xs text-stone-400">—</span>
                  )}
                </td>
                <td className="px-3 py-2 text-xs">
                  {s.inbox_score >= 0 ? Number(s.inbox_score).toFixed(0) : "—"}
                  {s.spam_checked_at ? (
                    <div className="text-[10px] text-stone-400">{s.spam_checked_at}</div>
                  ) : null}
                </td>
                <td className="px-3 py-2">{s.sent_count}</td>
                <td className="px-3 py-2">
                  <div className="flex flex-wrap gap-1">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={working}
                      onClick={async () => {
                        try {
                          await TestSmtp(s.id, "");
                          toast.success(t("smtps.toast.testOk", { id: s.id }));
                        } catch (e: any) {
                          toast.error(String(e?.message ?? e));
                        }
                      }}
                    >
                      {t("common.test")}
                    </Button>
                    <Button size="sm" variant="default" disabled={working} onClick={() => onAnalyze(s.id)}>
                      {analyzing === s.id ? t("smtps.analyzing") : t("smtps.check")}
                    </Button>
                    <Button size="sm" variant="secondary" disabled={working} onClick={() => onExtract(s.id)}>
                      {extracting === s.id ? t("smtps.extracting") : t("smtps.extract")}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={working}
                      onClick={async () => {
                        await SetSmtpActive(s.id, s.status !== "active");
                        refresh();
                      }}
                    >
                      {s.status === "active" ? t("common.disable") : t("common.enable")}
                    </Button>
                  </div>
                  {(analyzing === s.id || extracting === s.id) &&
                  (progress?.job === "analyze" || progress?.job === "extract-contacts") ? (
                    <div className="mt-2 max-w-xs">
                      <ProgressBar info={progress} />
                    </div>
                  ) : null}
                  {s.spam_summary ? (
                    <p className="mt-1 max-w-xs truncate font-mono text-[10px] text-stone-500" title={s.spam_summary}>
                      {s.spam_summary}
                    </p>
                  ) : null}
                </td>
              </tr>
            ))}
            {!list.length && (
              <tr>
                <td colSpan={7} className="px-3 py-8 text-center text-stone-500">
                  {t("smtps.empty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
    </div>
  );
}

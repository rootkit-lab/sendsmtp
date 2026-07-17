import { useCallback, useEffect, useState } from "react";
import { Dialogs, Events } from "@wailsio/runtime";
import {
  DeleteAllEmails,
  GetStatus,
  ImportEmailsFile,
  ImportEmailsText,
  ListEmailsPage,
  ResetFailed,
} from "../../bindings/github.com/wiz/sendsmtp/appservice";
import type { Email, StatusCounts } from "../../bindings/github.com/wiz/sendsmtp/internal/store/models";
import { Button } from "@/components/ui/button";
import { Badge, Input, Label, Textarea } from "@/components/ui/form";
import { ProgressBar, eventData, type ProgressInfo } from "@/components/ui/progress";
import { useTranslation } from "@/i18n";
import { toast } from "sonner";

const PAGE_SIZES = [50, 100, 200] as const;
const FILTERS = ["all", "pending", "sent", "failed", "sending"] as const;
/** Wails JS runtime chunks IPC bodies >512KB; Go alpha.88 can't reassemble → JSON EOF. Stay under. */
const MAX_PASTE_CHARS = 400_000;

function pasteTooLarge(text: string): boolean {
  return text.length > MAX_PASTE_CHARS;
}

export function EmailsPage() {
  const { t } = useTranslation();
  const [raw, setRaw] = useState("");
  const [filter, setFilter] = useState<(typeof FILTERS)[number]>("all");
  const [query, setQuery] = useState("");
  const [queryInput, setQueryInput] = useState("");
  const [list, setList] = useState<Email[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState<(typeof PAGE_SIZES)[number]>(50);
  const [counts, setCounts] = useState<StatusCounts | null>(null);
  const [busy, setBusy] = useState(false);
  const [validate, setValidate] = useState(true);
  const [progress, setProgress] = useState<ProgressInfo | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [pageRes, status] = await Promise.all([
        ListEmailsPage(filter, query, pageSize, page * pageSize),
        GetStatus(),
      ]);
      setList(pageRes?.items || []);
      setTotal(Number(pageRes?.total) || 0);
      setCounts(status);
      const maxPage = Math.max(0, Math.ceil((Number(pageRes?.total) || 0) / pageSize) - 1);
      if (page > maxPage) setPage(maxPage);
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    }
  }, [filter, query, page, pageSize]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    let raf = 0;
    let pending: ProgressInfo | null = null;
    const off = Events.On("job:progress", (ev: any) => {
      const data = eventData<ProgressInfo>(ev);
      if (!data || data.job !== "emails-import") return;
      if (data.done) {
        if (raf) cancelAnimationFrame(raf);
        raf = 0;
        pending = null;
        setProgress(null);
        return;
      }
      pending = data;
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        if (pending) setProgress(pending);
      });
    });
    return () => {
      if (raf) cancelAnimationFrame(raf);
      off?.();
    };
  }, []);

  const pageCount = Math.max(1, Math.ceil(total / pageSize) || 1);
  const from = total === 0 ? 0 : page * pageSize + 1;
  const to = Math.min(total, (page + 1) * pageSize);

  const filterCount = (f: string) => {
    if (!counts) return null;
    if (f === "all") return Number(counts.pending) + Number(counts.sending) + Number(counts.sent) + Number(counts.failed);
    if (f === "pending") return Number(counts.pending);
    if (f === "sending") return Number(counts.sending);
    if (f === "sent") return Number(counts.sent);
    if (f === "failed") return Number(counts.failed);
    return null;
  };

  const importFromFile = useCallback(async () => {
    try {
      const picked = await Dialogs.OpenFile({
        Title: t("emails.dialogTitle"),
        CanChooseFiles: true,
        AllowsMultipleSelection: false,
        Filters: [
          { DisplayName: t("emails.filterText"), Pattern: "*.txt;*.csv;*.list" },
          { DisplayName: t("emails.filterAll"), Pattern: "*.*" },
        ],
      });
      const path = Array.isArray(picked) ? picked[0] : picked;
      if (!path?.trim()) return;
      setBusy(true);
      setProgress({
        job: "emails-import",
        phase: "start",
        percent: 1,
        message: validate ? t("emails.validatingFile") : t("emails.importingFile"),
      });
      const res = await ImportEmailsFile(path, validate);
      toast.success(
        t("emails.toast.fileResult", {
          inserted: res.inserted,
          skipped: res.skipped,
          invalid: res.invalid ? t("emails.toast.invalidPart", { n: res.invalid }) : "",
          total: res.total,
        })
      );
      setPage(0);
      await refresh();
    } catch (e: any) {
      const msg = String(e?.message ?? e);
      toast.error(
        msg.includes("JSON") || msg.includes("runtime call") ? t("emails.toast.ipcFail") : msg
      );
    } finally {
      setBusy(false);
      setProgress(null);
    }
  }, [validate, refresh, t]);

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-display)] text-3xl">{t("emails.title")}</h1>
        <p className="mt-1 text-stone-500">{t("emails.subtitle")}</p>
      </header>

      <section className="space-y-3">
        <Label>{t("emails.importLabel")}</Label>
        <p className="text-xs text-stone-500">{t("emails.importHint")}</p>
        <Textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          placeholder={"a@x.com\nb@y.com"}
          disabled={busy}
          className="min-h-[100px] max-h-[220px]"
        />
        <label className="flex items-center gap-2 text-sm text-stone-700">
          <input
            type="checkbox"
            checked={validate}
            disabled={busy}
            onChange={(e) => setValidate(e.target.checked)}
          />
          {t("emails.validate")}
        </label>
        <ProgressBar info={busy ? progress : null} />
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy || !raw.trim()}
            onClick={async () => {
              if (pasteTooLarge(raw)) {
                toast.error(t("emails.toast.pasteTooLarge"));
                void importFromFile();
                return;
              }
              setBusy(true);
              setProgress({
                job: "emails-import",
                phase: "start",
                percent: 1,
                message: validate ? t("emails.validating") : t("emails.importingShort"),
              });
              try {
                const res = await ImportEmailsText(raw, validate);
                toast.success(
                  t("emails.toast.pasteResult", {
                    inserted: res.inserted,
                    skipped: res.skipped,
                    invalid: res.invalid ? t("emails.toast.invalidPart", { n: res.invalid }) : "",
                    total: res.total,
                  })
                );
                setRaw("");
                setPage(0);
                await refresh();
              } catch (e: any) {
                const msg = String(e?.message ?? e);
                if (msg.includes("JSON") || msg.includes("runtime call")) {
                  toast.error(t("emails.toast.ipcPaste"));
                  void importFromFile();
                } else {
                  toast.error(msg);
                }
              } finally {
                setBusy(false);
                setProgress(null);
              }
            }}
          >
            {busy ? t("emails.importing") : t("emails.import")}
          </Button>
          <Button variant="secondary" disabled={busy} onClick={() => void importFromFile()}>
            {t("emails.importFile")}
          </Button>
          <Button
            variant="secondary"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              try {
                const n = await ResetFailed();
                toast.success(t("emails.toast.requeued", { n }));
                setFilter("pending");
                setPage(0);
                await refresh();
              } catch (e: any) {
                toast.error(String(e?.message ?? e));
              } finally {
                setBusy(false);
              }
            }}
          >
            {t("emails.resetFailed")}
          </Button>
          <Button
            variant="danger"
            disabled={busy}
            onClick={async () => {
              if (!window.confirm(t("emails.confirmDelete"))) {
                return;
              }
              setBusy(true);
              try {
                const n = await DeleteAllEmails();
                toast.success(t("emails.toast.deleted", { n }));
                setPage(0);
                await refresh();
              } catch (e: any) {
                toast.error(String(e?.message ?? e));
              } finally {
                setBusy(false);
              }
            }}
          >
            {t("emails.deleteAll")}
          </Button>
        </div>
      </section>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-2">
          {FILTERS.map((f) => {
            const n = filterCount(f);
            return (
              <Button
                key={f}
                size="sm"
                variant={filter === f ? "default" : "outline"}
                onClick={() => {
                  setFilter(f);
                  setPage(0);
                }}
              >
                {t(`common.${f}` as "common.all")}
                {n != null ? ` (${n})` : ""}
              </Button>
            );
          })}
        </div>
        <form
          className="flex min-w-[220px] max-w-sm flex-1 gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setQuery(queryInput.trim());
            setPage(0);
          }}
        >
          <Input
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
            placeholder={t("emails.searchPlaceholder")}
            disabled={busy}
          />
          <Button type="submit" size="sm" variant="secondary" disabled={busy}>
            {t("common.search")}
          </Button>
        </form>
      </div>

      <section className="overflow-hidden rounded-lg border border-stone-300/80 bg-white/70">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-stone-300 bg-stone-100/80 text-xs uppercase text-stone-500">
            <tr>
              <th className="px-3 py-2">{t("common.email")}</th>
              <th className="px-3 py-2">{t("common.status")}</th>
              <th className="px-3 py-2">{t("common.attempts")}</th>
              <th className="px-3 py-2">{t("common.errorCol")}</th>
            </tr>
          </thead>
          <tbody>
            {list.map((e) => (
              <tr key={e.id} className="border-b border-stone-200/80">
                <td className="px-3 py-2 font-mono text-xs">{e.address}</td>
                <td className="px-3 py-2">
                  <Badge
                    tone={
                      e.status === "sent" ? "ok" : e.status === "failed" ? "danger" : e.status === "pending" ? "accent" : "warn"
                    }
                  >
                    {t(`common.${e.status}` as "common.pending")}
                  </Badge>
                </td>
                <td className="px-3 py-2">{e.attempts}</td>
                <td className="max-w-xs truncate px-3 py-2 font-mono text-xs text-red-800">{e.error}</td>
              </tr>
            ))}
            {!list.length && (
              <tr>
                <td colSpan={4} className="px-3 py-8 text-center text-stone-500">
                  {t("emails.empty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-stone-200 px-3 py-2 text-sm text-stone-600">
          <span>
            {total === 0
              ? t("emails.resultsZero")
              : t("emails.results", { from, to, total })}
          </span>
          <div className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-1.5 text-xs text-stone-500">
              {t("emails.perPage")}
              <select
                className="rounded border border-stone-300 bg-white px-1.5 py-1 text-sm"
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value) as (typeof PAGE_SIZES)[number]);
                  setPage(0);
                }}
              >
                {PAGE_SIZES.map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            <Button size="sm" variant="outline" disabled={page <= 0 || busy} onClick={() => setPage((p) => Math.max(0, p - 1))}>
              {t("common.prev")}
            </Button>
            <span className="min-w-[4.5rem] text-center text-xs tabular-nums">
              {page + 1}/{pageCount}
            </span>
            <Button
              size="sm"
              variant="outline"
              disabled={page + 1 >= pageCount || busy}
              onClick={() => setPage((p) => p + 1)}
            >
              {t("common.next")}
            </Button>
          </div>
        </div>
      </section>
    </div>
  );
}

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
import { toast } from "sonner";

const PAGE_SIZES = [50, 100, 200] as const;
const FILTERS = ["all", "pending", "sent", "failed", "sending"] as const;
/** Wails JS runtime chunks IPC bodies >512KB; Go alpha.88 can't reassemble → JSON EOF. Stay under. */
const MAX_PASTE_CHARS = 400_000;

function pasteTooLarge(text: string): boolean {
  return text.length > MAX_PASTE_CHARS;
}

export function EmailsPage() {
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
        Title: "Importar lista de emails",
        CanChooseFiles: true,
        AllowsMultipleSelection: false,
        Filters: [
          { DisplayName: "Texto", Pattern: "*.txt;*.csv;*.list" },
          { DisplayName: "Todos", Pattern: "*.*" },
        ],
      });
      const path = Array.isArray(picked) ? picked[0] : picked;
      if (!path?.trim()) return;
      setBusy(true);
      setProgress({
        job: "emails-import",
        phase: "start",
        percent: 1,
        message: validate ? "Validando arquivo…" : "Importando arquivo…",
      });
      const res = await ImportEmailsFile(path, validate);
      toast.success(
        `Arquivo: ${res.inserted} inseridos, ${res.skipped} ignorados` +
          (res.invalid ? `, ${res.invalid} inválidos` : "") +
          ` · total ${res.total}`
      );
      setPage(0);
      await refresh();
    } catch (e: any) {
      const msg = String(e?.message ?? e);
      toast.error(
        msg.includes("JSON") || msg.includes("runtime call")
          ? "Falha no diálogo/IPC — reinicie o app (task dev) e tente Importar arquivo de novo"
          : msg
      );
    } finally {
      setBusy(false);
      setProgress(null);
    }
  }, [validate, refresh]);

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-display)] text-3xl">Emails</h1>
        <p className="mt-1 text-stone-500">Lista de destinatários e fila — um endereço único por mailbox</p>
      </header>

      <section className="space-y-3">
        <Label>Importar (1 por linha)</Label>
        <p className="text-xs text-stone-500">
          Listas grandes: use Importar arquivo (só o caminho passa pelo IPC). Colar &gt;~400KB quebra o runtime.
        </p>
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
          Validar (sintaxe + MX real + bloqueia descartáveis) — inválidos e duplicados não entram
        </label>
        <ProgressBar info={busy ? progress : null} />
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy || !raw.trim()}
            onClick={async () => {
              if (pasteTooLarge(raw)) {
                toast.error("Lista grande demais para colar — use Importar arquivo");
                void importFromFile();
                return;
              }
              setBusy(true);
              setProgress({
                job: "emails-import",
                phase: "start",
                percent: 1,
                message: validate ? "Iniciando validação…" : "Importando…",
              });
              try {
                const res = await ImportEmailsText(raw, validate);
                toast.success(
                  `${res.inserted} inseridos, ${res.skipped} ignorados (já existiam / dup)` +
                    (res.invalid ? `, ${res.invalid} inválidos` : "") +
                    ` · total ${res.total}`
                );
                setRaw("");
                setPage(0);
                await refresh();
              } catch (e: any) {
                const msg = String(e?.message ?? e);
                if (msg.includes("JSON") || msg.includes("runtime call")) {
                  toast.error("Paste grande demais para o IPC — use Importar arquivo");
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
            {busy ? "Importando…" : "Importar"}
          </Button>
          <Button variant="secondary" disabled={busy} onClick={() => void importFromFile()}>
            Importar arquivo
          </Button>
          <Button
            variant="secondary"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              try {
                const n = await ResetFailed();
                toast.success(`${n} reenfileirados`);
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
            Reset failed
          </Button>
          <Button
            variant="danger"
            disabled={busy}
            onClick={async () => {
              if (!window.confirm("Apagar TODOS os destinatários da lista? Esta ação não pode ser desfeita.")) {
                return;
              }
              setBusy(true);
              try {
                const n = await DeleteAllEmails();
                toast.success(`${n} emails apagados`);
                setPage(0);
                await refresh();
              } catch (e: any) {
                toast.error(String(e?.message ?? e));
              } finally {
                setBusy(false);
              }
            }}
          >
            Apagar todos
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
                {f}
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
            placeholder="Buscar endereço…"
            disabled={busy}
          />
          <Button type="submit" size="sm" variant="secondary" disabled={busy}>
            Buscar
          </Button>
        </form>
      </div>

      <section className="overflow-hidden rounded-lg border border-stone-300/80 bg-white/70">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-stone-300 bg-stone-100/80 text-xs uppercase text-stone-500">
            <tr>
              <th className="px-3 py-2">Email</th>
              <th className="px-3 py-2">Status</th>
              <th className="px-3 py-2">Attempts</th>
              <th className="px-3 py-2">Error</th>
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
                    {e.status}
                  </Badge>
                </td>
                <td className="px-3 py-2">{e.attempts}</td>
                <td className="max-w-xs truncate px-3 py-2 font-mono text-xs text-red-800">{e.error}</td>
              </tr>
            ))}
            {!list.length && (
              <tr>
                <td colSpan={4} className="px-3 py-8 text-center text-stone-500">
                  Nenhum email
                </td>
              </tr>
            )}
          </tbody>
        </table>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-stone-200 px-3 py-2 text-sm text-stone-600">
          <span>
            {total === 0 ? "0 resultados" : `${from}–${to} de ${total}`}
          </span>
          <div className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-1.5 text-xs text-stone-500">
              Por página
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
              Anterior
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
              Próxima
            </Button>
          </div>
        </div>
      </section>
    </div>
  );
}

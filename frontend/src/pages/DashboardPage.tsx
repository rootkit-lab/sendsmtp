import { useCallback, useEffect, useMemo, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  ClearErrorLogs,
  GetStats,
  PauseCampaign,
  ReenableSMTPs,
  ResetFailed,
  ResumeCampaign,
  StartCampaign,
} from "../../bindings/github.com/wiz/sendsmtp/appservice";
import type { Stats } from "../../bindings/github.com/wiz/sendsmtp/internal/engine/models";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/form";
import { toast } from "sonner";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

type Progress = {
  sent?: number;
  failed?: number;
  pending?: number;
  rate?: number;
  state?: string;
};

const PIE_COLORS = ["#0f766e", "#a16207", "#b91c1c"];

export function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [live, setLive] = useState<Progress>({});
  const [busy, setBusy] = useState(false);
  const [localRate, setLocalRate] = useState<{ t: number; rate: number; sent: number }[]>([]);

  const refresh = useCallback(async () => {
    try {
      const s = await GetStats();
      setStats(s);
      if (s?.rate_history?.length) {
        setLocalRate(
          s.rate_history.map((p) => ({
            t: Number(p.t) || 0,
            rate: Number(p.rate) || 0,
            sent: Number(p.sent) || 0,
          }))
        );
      }
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    }
  }, []);

  useEffect(() => {
    refresh();
    // Live counts come from campaign:progress; avoid hammering SQLite every 1.5s.
    const t = setInterval(refresh, 5000);
    const offProgress = Events.On("campaign:progress", (ev: any) => {
      const data = Array.isArray(ev?.data) ? ev.data[0] : ev?.data ?? ev;
      setLive(data || {});
      if (data) {
        setLocalRate((prev) => {
          const next = [
            ...prev,
            {
              t: Date.now(),
              rate: Number(data.rate) || 0,
              sent: Number(data.sent) || 0,
            },
          ];
          return next.slice(-80);
        });
      }
    });
    const offDone = Events.On("campaign:done", () => {
      toast.success("Campanha concluída");
      refresh();
    });
    return () => {
      clearInterval(t);
      offProgress?.();
      offDone?.();
    };
  }, [refresh]);

  const status = stats?.status;
  const pending = live.pending ?? status?.pending ?? 0;
  const sent = live.sent ?? status?.sent ?? 0;
  const failed = live.failed ?? status?.failed ?? 0;
  const rate = live.rate ?? stats?.rate ?? 0;
  const running = Boolean(stats?.running) || live.state === "running";
  const state = running ? "running" : live.state ?? stats?.campaign?.state ?? "idle";

  const pieData = useMemo(
    () => [
      { name: "Sent", value: sent },
      { name: "Pending", value: pending },
      { name: "Failed", value: failed },
    ],
    [sent, pending, failed]
  );

  const smtpBars = useMemo(
    () =>
      (stats?.smtp_stats || []).map((s) => ({
        name: `${s.host}`.replace(/^mail\.|^smtp\./, "").slice(0, 18),
        sent: Number(s.sent_count) || 0,
        fails: Number(s.fail_count) || 0,
        status: s.status,
      })),
    [stats]
  );

  async function run(action: () => Promise<void>, ok: string) {
    setBusy(true);
    try {
      await action();
      toast.success(ok);
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-[family-name:var(--font-display)] text-3xl text-stone-900">Dashboard</h1>
          <p className="mt-1 text-stone-500">Throughput ao vivo, saúde dos SMTPs e controle da fila</p>
        </div>
        <Badge tone={state === "running" ? "ok" : state === "paused" ? "warn" : "neutral"}>{state}</Badge>
      </header>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Metric label="Pending" value={pending} />
        <Metric label="Sent" value={sent} />
        <Metric label="Failed" value={failed} />
        <Metric label="Rate" value={`${Number(rate).toFixed(1)}/s`} />
      </div>

      <div className="flex flex-wrap gap-2">
        <Button disabled={busy || running} onClick={() => run(StartCampaign, "Campanha iniciada")}>
          Start
        </Button>
        <Button variant="secondary" disabled={busy || !running} onClick={() => run(PauseCampaign, "Pausado")}>
          Pause
        </Button>
        <Button variant="outline" disabled={busy || running} onClick={() => run(ResumeCampaign, "Retomado")}>
          Resume
        </Button>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="secondary"
          disabled={busy}
          onClick={() =>
            run(async () => {
              const n = await ResetFailed();
              if (!n) throw new Error("Nenhum failed para resetar");
            }, "Failed reenfileirados")
          }
        >
          Reset failed
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={() => run(async () => { await ReenableSMTPs(); }, "SMTPs reativados")}
        >
          Reativar SMTPs
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={busy}
          onClick={() => run(async () => { await ClearErrorLogs(); }, "Logs limpos")}
        >
          Limpar logs
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <section className="rounded-lg border border-stone-300/80 bg-white/70 p-4">
          <h2 className="mb-3 text-sm font-semibold text-stone-800">Fila</h2>
          <div className="h-56">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie data={pieData} dataKey="value" nameKey="name" innerRadius={50} outerRadius={80} paddingAngle={2}>
                  {pieData.map((_, i) => (
                    <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-1 flex justify-center gap-4 text-xs text-stone-600">
            <span>Sent {sent}</span>
            <span>Pending {pending}</span>
            <span>Failed {failed}</span>
          </div>
        </section>

        <section className="rounded-lg border border-stone-300/80 bg-white/70 p-4">
          <h2 className="mb-3 text-sm font-semibold text-stone-800">Taxa (emails/s)</h2>
          <div className="h-56">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={localRate}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e7e5e4" />
                <XAxis dataKey="t" hide />
                <YAxis width={36} tick={{ fontSize: 11 }} />
                <Tooltip labelFormatter={() => "rate"} />
                <Area type="monotone" dataKey="rate" stroke="#0f766e" fill="#99f6e4" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </section>
      </div>

      <section className="rounded-lg border border-stone-300/80 bg-white/70 p-4">
        <h2 className="mb-3 text-sm font-semibold text-stone-800">Envios por SMTP</h2>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={smtpBars}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e7e5e4" />
              <XAxis dataKey="name" tick={{ fontSize: 10 }} interval={0} angle={-20} textAnchor="end" height={50} />
              <YAxis width={36} tick={{ fontSize: 11 }} />
              <Tooltip />
              <Bar dataKey="sent" fill="#0f766e" name="sent" radius={[4, 4, 0, 0]} />
              <Bar dataKey="fails" fill="#b91c1c" name="fails" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
        <p className="mt-2 text-sm text-stone-600">
          Ativos: <strong>{status?.smtps_active ?? 0}</strong> · Desabilitados:{" "}
          <strong>{status?.smtps_disabled ?? 0}</strong>
        </p>
      </section>

      {stats?.top_errors?.length ? (
        <section className="rounded-lg border border-stone-300/80 bg-white/60 p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-stone-500">Top erros</h3>
          <div className="mt-2 space-y-1">
            {stats.top_errors.map((e) => (
              <p key={e} className="font-mono text-xs text-red-800">
                {e}
              </p>
            ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-stone-300/80 bg-white/70 px-4 py-3">
      <div className="text-xs uppercase tracking-wide text-stone-500">{label}</div>
      <div className="mt-1 font-[family-name:var(--font-display)] text-2xl text-stone-900">{value}</div>
    </div>
  );
}

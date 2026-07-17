import { cn } from "@/lib/utils";

export type ProgressInfo = {
  job?: string;
  phase?: string;
  current?: number;
  total?: number;
  percent?: number;
  message?: string;
  smtp_id?: number;
  done?: boolean;
};

export function ProgressBar({
  info,
  className,
}: {
  info: ProgressInfo | null;
  className?: string;
}) {
  if (!info || info.done) return null;
  const pct = Math.max(0, Math.min(100, Number(info.percent) || 0));
  const label =
    info.message ||
    (info.total
      ? `${info.phase || "…"} ${info.current ?? 0}/${info.total}`
      : info.phase || "Processando…");

  return (
    <div className={cn("space-y-1.5 rounded-md border border-stone-300/80 bg-white/70 p-3", className)}>
      <div className="flex items-center justify-between gap-3 text-xs text-stone-600">
        <span className="truncate">{label}</span>
        <span className="shrink-0 font-mono tabular-nums">{pct.toFixed(0)}%</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-stone-200">
        <div
          className="h-full rounded-full bg-teal-700 transition-[width] duration-300 ease-out"
          style={{ width: `${pct}%` }}
        />
      </div>
      {info.total != null && info.total > 0 ? (
        <p className="font-mono text-[10px] text-stone-400">
          {info.current ?? 0}/{info.total}
          {info.phase ? ` · ${info.phase}` : ""}
        </p>
      ) : null}
    </div>
  );
}

/** Normalize Wails Events.On payload shapes. */
export function eventData<T = any>(ev: any): T {
  return (Array.isArray(ev?.data) ? ev.data[0] : ev?.data ?? ev) as T;
}

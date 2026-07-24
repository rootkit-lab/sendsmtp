import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  DeleteServer,
  DeployAllServers,
  DeployServer,
  ImportServersText,
  ListServers,
  SetServerActive,
  TestServer,
} from "../../bindings/github.com/wiz/sendsmtp/appservice";
import type { Server } from "../../bindings/github.com/wiz/sendsmtp/internal/store/models";
import { Button } from "@/components/ui/button";
import { Badge, Label, Textarea } from "@/components/ui/form";
import { ProgressBar, eventData, type ProgressInfo } from "@/components/ui/progress";
import { useTranslation } from "@/i18n";
import { toast } from "sonner";

function statusTone(status: string): "ok" | "warn" | "danger" | "neutral" {
  switch (status) {
    case "active":
      return "ok";
    case "pending":
      return "warn";
    case "error":
    case "disabled":
      return "danger";
    default:
      return "neutral";
  }
}

export function ServersPage() {
  const { t } = useTranslation();
  const [raw, setRaw] = useState("");
  const [list, setList] = useState<Server[]>([]);
  const [busy, setBusy] = useState(false);
  const [deploying, setDeploying] = useState<number | null>(null);
  const [deployingAll, setDeployingAll] = useState(false);
  const [progress, setProgress] = useState<ProgressInfo | null>(null);

  const refresh = useCallback(async () => {
    try {
      setList((await ListServers()) || []);
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
      if (!data || data.job !== "server-deploy") return;
      setProgress(data.done ? null : data);
    });
    return () => {
      off?.();
    };
  }, []);

  const working = busy || deploying !== null || deployingAll;

  async function onImport() {
    setBusy(true);
    try {
      const res = await ImportServersText(raw);
      toast.success(
        t("servers.toast.imported", {
          inserted: res.inserted,
          updated: res.updated,
          invalid: res.invalid ? t("servers.toast.invalidPart", { n: res.invalid }) : "",
        })
      );
      setRaw("");
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  async function onDeploy(id: number) {
    setDeploying(id);
    setProgress({ job: "server-deploy", phase: "ssh", percent: 5, message: t("servers.progress.one", { id }) });
    try {
      const res = await DeployServer(id);
      toast.success(res.message || t("servers.toast.deployOk", { host: res.host, port: res.port }));
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
      await refresh();
    } finally {
      setDeploying(null);
      setProgress(null);
    }
  }

  async function onDeployAll() {
    setDeployingAll(true);
    setProgress({
      job: "server-deploy",
      phase: "batch",
      percent: 2,
      message: t("servers.progress.all", { n: list.length }),
    });
    try {
      const results = (await DeployAllServers()) || [];
      const ok = results.filter((r) => r.ok).length;
      toast.success(t("servers.toast.deployAll", { ok, total: results.length }));
      await refresh();
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setDeployingAll(false);
      setProgress(null);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-display)] text-3xl">{t("servers.title")}</h1>
        <p className="mt-1 text-stone-500">{t("servers.subtitle")}</p>
      </header>

      <section className="space-y-3">
        <Label>{t("servers.pasteLabel")}</Label>
        <p className="text-xs text-stone-500">{t("servers.pasteHint")}</p>
        <Textarea
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          placeholder={t("servers.placeholder")}
          className="min-h-[160px] font-mono text-xs"
          disabled={working}
        />
        <ProgressBar info={working ? progress : null} />
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || !raw.trim() || working} onClick={onImport}>
            {busy ? t("servers.importing") : t("servers.import")}
          </Button>
          <Button variant="secondary" disabled={working || !list.length} onClick={onDeployAll}>
            {deployingAll ? t("servers.deployAllBusy") : t("servers.deployAll")}
          </Button>
        </div>
      </section>

      <section className="overflow-x-auto overflow-hidden rounded-lg border border-stone-300/80 bg-white/70">
        <table className="w-full min-w-[820px] text-left text-sm">
          <thead className="border-b border-stone-300 bg-stone-100/80 text-xs uppercase text-stone-500">
            <tr>
              <th className="px-3 py-2">{t("servers.col.host")}</th>
              <th className="px-3 py-2">{t("servers.col.proxy")}</th>
              <th className="px-3 py-2">{t("common.status")}</th>
              <th className="px-3 py-2">{t("common.sent")}</th>
              <th className="px-3 py-2">{t("common.actions")}</th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => (
              <tr key={s.id} className="border-b border-stone-200/80 align-top">
                <td className="px-3 py-2 font-mono text-xs">
                  {s.ssh_user}@{s.host}:{s.ssh_port}
                  <div className="mt-0.5 text-stone-400">prefer :{s.prefer_port}</div>
                </td>
                <td className="px-3 py-2 font-mono text-xs">
                  {s.proxy_port > 0 ? (
                    <>
                      {s.host}:{s.proxy_port}
                      <div className="mt-0.5 text-stone-400">{s.proxy_user}</div>
                    </>
                  ) : (
                    <span className="text-stone-400">—</span>
                  )}
                </td>
                <td className="px-3 py-2">
                  <Badge tone={statusTone(s.status)}>{s.status}</Badge>
                  {s.last_error ? (
                    <p className="mt-1 max-w-xs truncate text-xs text-red-700" title={s.last_error}>
                      {s.last_error}
                    </p>
                  ) : null}
                </td>
                <td className="px-3 py-2 tabular-nums">{s.sent_count}</td>
                <td className="px-3 py-2">
                  <div className="flex flex-wrap gap-1">
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={working}
                      onClick={() => onDeploy(s.id)}
                    >
                      {deploying === s.id ? t("servers.deploying") : t("servers.deploy")}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={working || s.proxy_port <= 0}
                      onClick={async () => {
                        try {
                          await TestServer(s.id);
                          toast.success(t("servers.toast.testOk", { id: s.id }));
                          await refresh();
                        } catch (e: any) {
                          toast.error(String(e?.message ?? e));
                          await refresh();
                        }
                      }}
                    >
                      {t("common.test")}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={working}
                      onClick={async () => {
                        try {
                          await SetServerActive(s.id, s.status !== "active");
                          await refresh();
                        } catch (e: any) {
                          toast.error(String(e?.message ?? e));
                        }
                      }}
                    >
                      {s.status === "active" ? t("common.disable") : t("common.enable")}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={working}
                      onClick={async () => {
                        try {
                          await DeleteServer(s.id);
                          await refresh();
                        } catch (e: any) {
                          toast.error(String(e?.message ?? e));
                        }
                      }}
                    >
                      {t("servers.delete")}
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
            {!list.length ? (
              <tr>
                <td colSpan={5} className="px-3 py-8 text-center text-stone-400">
                  {t("servers.empty")}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </section>
    </div>
  );
}

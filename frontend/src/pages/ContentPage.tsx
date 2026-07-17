import { useEffect, useMemo, useState } from "react";
import Editor from "@monaco-editor/react";
import {
  GetHtml,
  GetLinks,
  GetSubjects,
  ImportLinksText,
  ImportSubjectsText,
  SetHtml,
} from "../../bindings/github.com/wiz/sendsmtp/appservice";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/form";
import { toast } from "sonner";

function firstLine(text: string, fallback: string) {
  const line = text
    .split("\n")
    .map((l) => l.trim())
    .find((l) => l && !l.startsWith("#"));
  return line || fallback;
}

/** Expand {a|b|c} spintax (innermost first). */
function spin(input: string): string {
  let s = input;
  for (let guard = 0; guard < 500; guard++) {
    let start = -1;
    let depth = 0;
    let found = -1;
    let end = -1;
    for (let i = 0; i < s.length; i++) {
      const ch = s[i];
      if (ch === "{") {
        if (depth === 0) start = i;
        depth++;
      } else if (ch === "}") {
        if (depth === 0) continue;
        depth--;
        if (depth === 0 && start >= 0) {
          const inner = s.slice(start + 1, i);
          if (inner.includes("|")) {
            found = start;
            end = i;
            break;
          }
          start = -1;
        }
      }
    }
    if (found < 0) break;
    const inner = s.slice(found + 1, end);
    const parts: string[] = [];
    let d = 0;
    let p0 = 0;
    for (let i = 0; i < inner.length; i++) {
      if (inner[i] === "{") d++;
      else if (inner[i] === "}") d = Math.max(0, d - 1);
      else if (inner[i] === "|" && d === 0) {
        parts.push(inner.slice(p0, i));
        p0 = i + 1;
      }
    }
    parts.push(inner.slice(p0));
    const pick = parts[Math.floor(Math.random() * parts.length)] ?? "";
    s = s.slice(0, found) + pick + s.slice(end + 1);
  }
  return s;
}

function uniq() {
  return Math.random().toString(16).slice(2, 10);
}

function applyPreview(html: string, subjects: string, links: string) {
  const assuntoRaw = firstLine(subjects, "Assunto demo");
  const linkBase = firstLine(links, "https://example.com");
  const email = "demo@example.com";
  const link = personalizeLink(linkBase, email);
  const from = "info@example.com";
  const fromBit = from ? ` · ${from}` : "";
  const id = uniq();

  const replaceCommon = (s: string, fromValue: string) =>
    s
      .split("<span data-from>{{from}}</span>")
      .join(fromValue === "" ? "" : fromBit)
      .split("{{email}}")
      .join(email)
      .split("{{link}}")
      .join(link)
      .split("{{from}}")
      .join(fromValue)
      .split("{{uniq}}")
      .join(id)
      .split("{{id}}")
      .join(id);

  const assunto = spin(replaceCommon(assuntoRaw, from));
  const body = spin(
    replaceCommon(html, from)
      .split("{{assunto}}")
      .join(assunto)
      .split("{{subject}}")
      .join(assunto)
  );
  return { html: body, subject: assunto };
}

/** Mirrors mailer.PersonalizeLink: base/?p=<email> */
function personalizeLink(base: string, email: string): string {
  const b = base.trim();
  const e = email.trim();
  if (!b) return "";
  if (!e) return b;
  try {
    const u = new URL(b);
    u.searchParams.set("p", e);
    if (!u.pathname) u.pathname = "/";
    return u.toString();
  } catch {
    return `${b.replace(/\/+$/, "")}/?p=${encodeURIComponent(e)}`;
  }
}

const monacoOpts = {
  minimap: { enabled: false },
  fontSize: 13,
  fontFamily: "IBM Plex Mono, ui-monospace, monospace",
  wordWrap: "on" as const,
  scrollBeyondLastLine: false,
  automaticLayout: true,
  tabSize: 2,
  padding: { top: 12, bottom: 12 },
};

export function ContentPage() {
  const [subjects, setSubjects] = useState("");
  const [links, setLinks] = useState("");
  const [html, setHtml] = useState("");
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const [spinKey, setSpinKey] = useState(0);

  useEffect(() => {
    (async () => {
      try {
        setSubjects(((await GetSubjects()) || []).join("\n"));
        setLinks(((await GetLinks()) || []).join("\n"));
        setHtml((await GetHtml()) || "");
      } catch (e: any) {
        toast.error(String(e?.message ?? e));
      } finally {
        setLoaded(true);
      }
    })();
  }, []);

  const preview = useMemo(() => applyPreview(html, subjects, links), [html, subjects, links, spinKey]);
  const previewHtml = preview.html;
  const previewSubject = preview.subject;

  async function save() {
    setBusy(true);
    try {
      await ImportSubjectsText(subjects);
      await ImportLinksText(links);
      await SetHtml(html);
      toast.success("Conteúdo salvo");
    } catch (e: any) {
      toast.error(String(e?.message ?? e));
    } finally {
      setBusy(false);
    }
  }

  if (!loaded) {
    return <p className="text-stone-500">Carregando conteúdo…</p>;
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-[family-name:var(--font-display)] text-3xl">Conteúdo</h1>
          <p className="mt-1 text-stone-500">
            Monaco + preview · {"{{email}}"} {"{{link}}"} {"{{assunto}}"} {"{{from}}"} {"{{uniq}}"} · spintax{" "}
            {"{a|b|c}"} (precisa de |)
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" disabled={busy} onClick={() => setSpinKey((k) => k + 1)}>
            Re-spin preview
          </Button>
          <Button disabled={busy} onClick={save}>
            Salvar
          </Button>
        </div>
      </header>

      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <Label>Assuntos (1 por linha)</Label>
          <div className="mt-1 overflow-hidden rounded-md border border-stone-300">
            <Editor
              height="160px"
              defaultLanguage="plaintext"
              theme="vs"
              value={subjects}
              onChange={(v) => setSubjects(v ?? "")}
              options={monacoOpts}
            />
          </div>
        </div>
        <div>
          <Label>Links (1 por linha)</Label>
          <div className="mt-1 overflow-hidden rounded-md border border-stone-300">
            <Editor
              height="160px"
              defaultLanguage="plaintext"
              theme="vs"
              value={links}
              onChange={(v) => setLinks(v ?? "")}
              options={monacoOpts}
            />
          </div>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="min-w-0">
          <Label>HTML (Monaco)</Label>
          <div className="mt-1 overflow-hidden rounded-md border border-stone-300">
            <Editor
              height="420px"
              defaultLanguage="html"
              theme="vs"
              value={html}
              onChange={(v) => setHtml(v ?? "")}
              options={{
                ...monacoOpts,
                formatOnPaste: true,
                formatOnType: true,
              }}
            />
          </div>
        </div>

        <div className="min-w-0">
          <Label>Preview realtime</Label>
          <div className="mt-1 overflow-hidden rounded-md border border-stone-300 bg-white">
            <div className="border-b border-stone-200 bg-stone-50 px-3 py-2">
              <p className="truncate text-xs text-stone-500">Assunto</p>
              <p className="truncate text-sm font-medium text-stone-900">{previewSubject}</p>
            </div>
            <iframe
              title="email-preview"
              className="h-[380px] w-full bg-white"
              sandbox=""
              srcDoc={previewHtml || "<p style='padding:16px;color:#78716c'>Digite HTML para ver o preview…</p>"}
            />
          </div>
          <p className="mt-2 text-xs text-stone-500">
            Spintax {"{a|b|c}"} exige pipe. {"{{link}}"} no envio vira base/?p=email. From opcional:{" "}
            {"<span data-from>{{from}}</span>"}. Preview usa 1º assunto/link.
          </p>
        </div>
      </div>
    </div>
  );
}

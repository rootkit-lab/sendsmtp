(() => {
  const data = window.SENDSMTP_CAMPAIGNS;
  if (!data) return;

  /** Pick first spintax option for preview display only */
  function resolveSpin(text) {
    return String(text).replace(/\{([^{}|]+(?:\|[^{}|]+)*)\}/g, (_, inner) => {
      if (!inner.includes("|")) return `{${inner}}`;
      return inner.split("|")[0];
    });
  }

  function escapeAttr(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/"/g, "&quot;")
      .replace(/</g, "&lt;");
  }

  function buildEmailHtml(campaign, { preview = false } = {}) {
    const m = campaign.mail;
    const accent = campaign.accent;
    const spin = preview ? resolveSpin : (t) => t;
    const emailSample = preview ? "cliente@exemplo.com" : "{{email}}";
    const uniqSample = preview ? "7f3a9c12" : "{{uniq}}";
    const linkSample = preview ? "https://exemplo.com/doc" : "{{link}}";
    const fromSample = preview ? "noreply@empresa.com" : "{{from}}";

    const fill = (t) =>
      spin(t)
        .replaceAll("{{email}}", emailSample)
        .replaceAll("{{uniq}}", uniqSample)
        .replaceAll("{{link}}", linkSample)
        .replaceAll("{{from}}", fromSample)
        .replaceAll("{{assunto}}", spin(campaign.subjects[0] || ""));

    const rows = m.rows
      .map(([k, v], i) => {
        const border = i < m.rows.length - 1 ? "border-bottom:1px solid #e5e7eb;" : "";
        return `<tr>
          <td style="padding:12px 14px;font-size:12px;color:#6b7280;${border}width:42%">${fill(k)}</td>
          <td style="padding:12px 14px;font-size:13px;font-weight:600;color:#111827;text-align:right;${border}font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace">${fill(v)}</td>
        </tr>`;
      })
      .join("");

    return `<!DOCTYPE html>
<html lang="${escapeAttr(m.lang)}">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <meta name="x-msg-id" content="${preview ? uniqSample : "{{uniq}}"}" />
  <title>${preview ? fill(campaign.subjects[0] || campaign.title) : "{{assunto}}"}</title>
</head>
<body style="margin:0;padding:0;background:#f0f2f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#111827;-webkit-font-smoothing:antialiased;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f0f2f5;padding:40px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="520" cellspacing="0" cellpadding="0" style="max-width:520px;width:100%;">
          <tr>
            <td style="padding:0 0 16px;">
              <p style="margin:0;font-size:11px;letter-spacing:0.12em;text-transform:uppercase;color:#6b7280;font-weight:600;">
                ${fill(m.eyebrow)}
              </p>
            </td>
          </tr>
          <tr>
            <td style="background:#ffffff;border-radius:10px;overflow:hidden;border:1px solid #e5e7eb;">
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0">
                <tr>
                  <td style="height:4px;background:${accent};font-size:0;line-height:0;">&nbsp;</td>
                </tr>
              </table>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0">
                <tr>
                  <td style="padding:32px 28px 28px;">
                    <p style="margin:0 0 8px;font-size:12px;color:${accent};font-weight:600;letter-spacing:0.04em;text-transform:uppercase;">
                      ${fill(m.badge)}
                    </p>
                    <h1 style="margin:0 0 12px;font-size:22px;font-weight:650;letter-spacing:-0.025em;line-height:1.25;color:#111827;">
                      ${fill(m.heading)}
                    </h1>
                    <p style="margin:0 0 24px;font-size:14px;line-height:1.6;color:#6b7280;">
                      ${fill(m.intro)}
                      <br />
                      <span style="display:inline-block;margin-top:6px;color:#111827;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px;">${emailSample}</span>
                    </p>
                    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:0 0 24px;border:1px solid #e5e7eb;border-radius:8px;">
                      ${rows}
                    </table>
                    <p style="margin:0 0 16px;font-size:13px;line-height:1.5;color:#6b7280;">
                      ${fill(m.ctaHint)}
                    </p>
                    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:0 0 18px;">
                      <tr>
                        <td align="center" style="background:${accent};border-radius:8px;">
                          <a href="${linkSample}" style="display:block;padding:14px 22px;color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;letter-spacing:-0.01em;">
                            ${fill(m.cta)}
                          </a>
                        </td>
                      </tr>
                    </table>
                    <p style="margin:0;font-size:11px;line-height:1.55;color:#9ca3af;word-break:break-all;text-align:center;">
                      ${fill(m.linkAlt)}: <a href="${linkSample}" style="color:#6b7280;text-decoration:underline;">${linkSample}</a>
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:22px 8px 0;text-align:center;font-size:11px;line-height:1.65;color:#9ca3af;">
              ${fill(m.footer)}<br />
              ${fill(m.footer2)}
              <br /><span style="color:#d1d5db;">${fill(m.footer3)}</span>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`;
  }

  function toast(msg) {
    let el = document.getElementById("camp-toast");
    if (!el) {
      el = document.createElement("div");
      el.id = "camp-toast";
      el.className = "toast";
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.classList.add("show");
    clearTimeout(el._t);
    el._t = setTimeout(() => el.classList.remove("show"), 1600);
  }

  async function copyText(text, okMsg) {
    try {
      await navigator.clipboard.writeText(text);
      toast(okMsg);
    } catch {
      toast("Copy failed");
    }
  }

  function current() {
    const locale = document.getElementById("camp-locale").value;
    const type = document.querySelector('.type-pills button[aria-pressed="true"]')?.dataset.type || "payment";
    return { locale, type, campaign: data[locale][type], localeLabel: data[locale].label };
  }

  function render() {
    const { locale, type, campaign, localeLabel } = current();
    const htmlExport = buildEmailHtml(campaign, { preview: false });
    const htmlPreview = buildEmailHtml(campaign, { preview: true });

    document.getElementById("camp-title").textContent = campaign.title;
    document.getElementById("camp-meta").innerHTML =
      `<strong>${localeLabel}</strong> · <code>${locale}</code> · ${type === "payment" ? "payment" : "invoice"}`;

    const frame = document.getElementById("camp-frame");
    frame.srcdoc = htmlPreview;

    document.getElementById("camp-html").textContent = htmlExport;

    const list = document.getElementById("camp-subjects");
    list.innerHTML = "";
    campaign.subjects.forEach((s) => {
      const li = document.createElement("li");
      const span = document.createElement("span");
      span.textContent = s;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = "Copy";
      btn.addEventListener("click", () => copyText(s, "Subject copied"));
      li.append(span, btn);
      list.appendChild(li);
    });

    // stash for action buttons
    window.__camp = { htmlExport, subjects: campaign.subjects.join("\n") + "\n" };
  }

  function wire() {
    const localeSel = document.getElementById("camp-locale");
    Object.entries(data).forEach(([code, pack]) => {
      const opt = document.createElement("option");
      opt.value = code;
      opt.textContent = `${pack.label} (${code})`;
      localeSel.appendChild(opt);
    });
    localeSel.value = "pt-BR";

    localeSel.addEventListener("change", render);

    document.querySelectorAll(".type-pills button").forEach((btn) => {
      btn.addEventListener("click", () => {
        document.querySelectorAll(".type-pills button").forEach((b) => b.setAttribute("aria-pressed", "false"));
        btn.setAttribute("aria-pressed", "true");
        render();
      });
    });

    document.querySelectorAll(".camp-tabs button").forEach((btn) => {
      btn.addEventListener("click", () => {
        const tab = btn.dataset.tab;
        document.querySelectorAll(".camp-tabs button").forEach((b) => b.setAttribute("aria-selected", "false"));
        btn.setAttribute("aria-selected", "true");
        document.querySelectorAll(".camp-panel").forEach((p) => {
          p.hidden = p.id !== `panel-${tab}`;
        });
      });
    });

    document.getElementById("btn-copy-html").addEventListener("click", () => {
      copyText(window.__camp?.htmlExport || "", "HTML copied — paste into data/msg.html");
    });
    document.getElementById("btn-copy-subjects").addEventListener("click", () => {
      copyText(window.__camp?.subjects || "", "Subjects copied — paste into data/assuntos.txt");
    });
    document.getElementById("btn-download-html").addEventListener("click", () => {
      const { locale, type } = current();
      const blob = new Blob([window.__camp?.htmlExport || ""], { type: "text/html;charset=utf-8" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `msg-${type}-${locale}.html`;
      a.click();
      URL.revokeObjectURL(a.href);
      toast("Download started");
    });

    render();
  }

  document.addEventListener("DOMContentLoaded", wire);
})();

(() => {
  const dict = {
    en: {
      "nav.home": "Home",
      "nav.install": "Install",
      "nav.guide": "Guide",
      "nav.cli": "CLI",
      "nav.config": "Config",
      "nav.github": "GitHub",
      "hero.title": "SendSMTP",
      "hero.lede": "Desktop SMTP campaigns — Wails, React, and SQLite.",
      "hero.support":
        "Import SMTPs, manage large recipient queues, personalize HTML, and send with a shared Go engine.",
      "hero.install": "Install",
      "hero.docs": "Documentation",
      "hero.source": "Source",
      "home.features": "What it does",
      "home.f1t": "SMTP discovery",
      "home.f1d": "email;password or goscan blocks — MX maps to submission hosts and AUTH is verified.",
      "home.f2t": "Large lists",
      "home.f2d": "File import avoids Wails paste limits. Deduped UNIQUE addresses; optional MX validation.",
      "home.f3t": "IMAP extract",
      "home.f3d": "Pull contacts and credentials from INBOX/Sent, then queue and import as SMTPs.",
      "home.f4t": "Templates",
      "home.f4d": "Placeholders, spintax {a|b|c}, and personalized {{link}}?p=<email> at send time.",
      "footer.mit": "MIT License",
      "footer.apt": "Signed APT on this site",
      "copy": "Copy",
      "copied": "Copied",
      "side.onthis": "On this page",
      "side.pages": "Docs",
    },
    pt: {
      "nav.home": "Início",
      "nav.install": "Instalar",
      "nav.guide": "Guia",
      "nav.cli": "CLI",
      "nav.config": "Config",
      "nav.github": "GitHub",
      "hero.title": "SendSMTP",
      "hero.lede": "Campanhas SMTP no desktop — Wails, React e SQLite.",
      "hero.support":
        "Importe SMTPs, gerencie filas grandes, personalize HTML e envie com um engine Go compartilhado.",
      "hero.install": "Instalar",
      "hero.docs": "Documentação",
      "hero.source": "Código",
      "home.features": "O que faz",
      "home.f1t": "Descoberta SMTP",
      "home.f1d": "email;senha ou blocos goscan — MX mapeia o host de envio e a AUTH é verificada.",
      "home.f2t": "Listas grandes",
      "home.f2d": "Importação por arquivo evita limites do Wails. Endereços UNIQUE; validação MX opcional.",
      "home.f3t": "Extração IMAP",
      "home.f3d": "Contatos e credenciais de INBOX/Enviados entram na fila e viram SMTPs.",
      "home.f4t": "Templates",
      "home.f4d": "Placeholders, spintax {a|b|c} e {{link}}?p=<email> personalizado no envio.",
      "footer.mit": "Licença MIT",
      "footer.apt": "APT assinado neste site",
      "copy": "Copiar",
      "copied": "Copiado",
      "side.onthis": "Nesta página",
      "side.pages": "Docs",
    },
  };

  function lang() {
    const stored = localStorage.getItem("sendsmtp-docs-lang");
    if (stored === "en" || stored === "pt") return stored;
    return (navigator.language || "en").toLowerCase().startsWith("pt") ? "pt" : "en";
  }

  function setLang(next) {
    localStorage.setItem("sendsmtp-docs-lang", next);
    apply(next);
  }

  function apply(code) {
    const table = dict[code] || dict.en;
    document.documentElement.lang = code;
    document.querySelectorAll("[data-i18n]").forEach((el) => {
      const key = el.getAttribute("data-i18n");
      if (table[key]) el.textContent = table[key];
    });
    document.querySelectorAll(".lang button").forEach((btn) => {
      btn.setAttribute("aria-pressed", btn.dataset.lang === code ? "true" : "false");
    });
    document.querySelectorAll(".copy").forEach((btn) => {
      if (!btn.dataset.busy) btn.textContent = table.copy || "Copy";
    });
  }

  function wireLang() {
    document.querySelectorAll(".lang button").forEach((btn) => {
      btn.addEventListener("click", () => setLang(btn.dataset.lang));
    });
  }

  function wireCopy() {
    document.querySelectorAll(".pre-wrap").forEach((wrap) => {
      const pre = wrap.querySelector("pre");
      if (!pre) return;
      let btn = wrap.querySelector(".copy");
      if (!btn) {
        btn = document.createElement("button");
        btn.type = "button";
        btn.className = "copy";
        btn.textContent = dict[lang()].copy;
        wrap.appendChild(btn);
      }
      btn.addEventListener("click", async () => {
        const text = pre.innerText;
        try {
          await navigator.clipboard.writeText(text);
          btn.dataset.busy = "1";
          btn.textContent = dict[lang()].copied;
          setTimeout(() => {
            delete btn.dataset.busy;
            btn.textContent = dict[lang()].copy;
          }, 1200);
        } catch {
          /* ignore */
        }
      });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    wireLang();
    apply(lang());
    wireCopy();
  });
})();

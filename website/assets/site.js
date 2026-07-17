(() => {
  const dict = {
    en: {
      "nav.home": "Home",
      "nav.install": "Install",
      "nav.guide": "Guide",
      "nav.cli": "CLI",
      "nav.config": "Config",
      "nav.campaigns": "Campaigns",
      "nav.github": "GitHub",
      "hero.title": "SendSMTP",
      "hero.kicker": "Open source · Desktop · MIT",
      "hero.lede": "Send transactional email campaigns from your desktop.",
      "hero.support":
        "Discover working SMTPs, import huge recipient lists, personalize every message, and send — all on your machine.",
      "hero.install": "Get the app",
      "hero.docs": "Read the guide",
      "hero.examples": "See examples",
      "hero.source": "View source",
      "home.features": "Built for real campaigns",
      "home.featuresLede": "From credential import to personalized HTML — one desktop workflow.",
      "home.strip1": "Signed APT & MSI installs",
      "home.strip2": "Works offline on your PC",
      "home.strip3": "EN & PT interface",
      "home.f1t": "Find servers that actually send",
      "home.f1d": "Paste email;password or goscan blocks. We resolve MX, probe AUTH, and keep only what works.",
      "home.f2t": "Handle lists of any size",
      "home.f2d": "Import from file, auto-dedupe addresses, optionally validate MX — without freezing the UI.",
      "home.f3t": "Grow the queue from IMAP",
      "home.f3d": "Extract contacts and credentials from INBOX and Sent, then add them to the campaign.",
      "home.f4t": "Personalize every send",
      "home.f4d": "HTML templates, spintax {a|b|c}, and unique {{link}} tracking per recipient.",
      "home.ctaTitle": "Start in minutes",
      "home.ctaBody": "Install from the signed APT repo, grab a Windows MSI, or build from source.",
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
      "nav.campaigns": "Campanhas",
      "nav.github": "GitHub",
      "hero.title": "SendSMTP",
      "hero.kicker": "Open source · Desktop · MIT",
      "hero.lede": "Envie campanhas de e-mail transacional direto do desktop.",
      "hero.support":
        "Descubra SMTPs que funcionam, importe listas enormes, personalize cada mensagem e dispare — tudo na sua máquina.",
      "hero.install": "Baixar o app",
      "hero.docs": "Ler o guia",
      "hero.examples": "Ver exemplos",
      "hero.source": "Ver código",
      "home.features": "Feito para campanhas de verdade",
      "home.featuresLede": "Da importação de credenciais ao HTML personalizado — um fluxo só no desktop.",
      "home.strip1": "Instaladores APT e MSI assinados",
      "home.strip2": "Roda offline no seu PC",
      "home.strip3": "Interface EN e PT",
      "home.f1t": "Ache servidores que realmente enviam",
      "home.f1d": "Cole email;senha ou blocos goscan. Resolvemos MX, testamos AUTH e guardamos só o que autentica.",
      "home.f2t": "Lidar com listas de qualquer tamanho",
      "home.f2d": "Importe por arquivo, dedupe automático e validação MX opcional — sem travar a interface.",
      "home.f3t": "Ampliar a fila via IMAP",
      "home.f3d": "Extraia contatos e credenciais da INBOX e Enviados e junte à campanha.",
      "home.f4t": "Personalizar cada envio",
      "home.f4d": "Templates HTML, spintax {a|b|c} e {{link}} único por destinatário.",
      "home.ctaTitle": "Comece em minutos",
      "home.ctaBody": "Instale pelo APT assinado, baixe o MSI no Windows ou compile do código-fonte.",
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

#!/usr/bin/env python3
"""
Gerador de Links abre.ai - ULTRA FAST
aiohttp + asyncio: alta taxa de sucesso
✅ links_ok.txt   → links abre.ai/XXXX gerados com sucesso (tempo real)
❌ links_fail.txt → URLs que falharam (tempo real)

API: POST https://abre.ai/_/generate
     Headers: Content-Type: application/json
              X-Requested-With: XMLHttpRequest
     Body: {"url_translation": {"url": "URL", "token": ""}}
     Resp: {"data": {"attributes": {"shortenedUrl": "https://abre.ai/XXXX", "token": "XXXX"}}}
     Link: https://abre.ai/XXXX
"""

import asyncio
import json
import string
import random
import time
import sys
import os
import aiohttp
from aiohttp import DummyCookieJar

# ─── TEMPLATE DA URL DESTINO ──────────────────────────────────────────────────
TEMPLATE = "https://crystalsignalbr.com?[random][random]"

# ─── CONFIGURAÇÕES ────────────────────────────────────────────────────────────
CONCORRENCIA = 80      # API REST pura → suporta alta concorrência
TIMEOUT_S    = 15
MAX_RETRIES  = 3

# ─── ARQUIVOS DE SAÍDA ────────────────────────────────────────────────────────
ARQUIVO_OK   = "links_ok.txt"    # ✅ links abre.ai/XXXX
ARQUIVO_FAIL = "links_fail.txt"  # ❌ URLs que falharam

# ─── USER-AGENTS ──────────────────────────────────────────────────────────────
USER_AGENTS = [
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
    "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
]

# ─── CORES ANSI ───────────────────────────────────────────────────────────────
class C:
    R  = '\033[0m'
    B  = '\033[1m'
    CY = '\033[36m'
    GR = '\033[32m'
    RD = '\033[31m'
    YL = '\033[33m'
    BL = '\033[34m'
    MG = '\033[35m'

# ─── ESTADO GLOBAL ────────────────────────────────────────────────────────────
gerados  = 0
falhados = 0
lock     = None

def rand_str(n=12):
    return "".join(random.choices(string.ascii_letters + string.digits, k=n))

def gerar_url_destino():
    url = TEMPLATE.replace("[random]", rand_str(12), 1)
    url = url.replace("[random]", rand_str(12), 1)
    return url

def fmt_tempo(s):
    h = int(s // 3600)
    m = int((s % 3600) // 60)
    seg = int(s % 60)
    return f"{h:02d}:{m:02d}:{seg:02d}"

def barra(pct, w=38):
    p = int(pct * w / 100)
    return f"[{'█'*p}{'░'*(w-p)}]"

def salvar_linha(caminho: str, linha: str):
    """Grava imediatamente no arquivo (tempo real, sem buffer)."""
    with open(caminho, "a", encoding="utf-8") as f:
        f.write(linha + "\n")
        f.flush()
        os.fsync(f.fileno())

# ─── WORKER ───────────────────────────────────────────────────────────────────
async def gerar_link(session: aiohttp.ClientSession, url_destino: str):
    global gerados, falhados

    for tentativa in range(MAX_RETRIES + 1):
        try:
            payload = json.dumps({"url_translation": {"url": url_destino, "token": ""}})
            headers = {
                "Content-Type": "application/json",
                "Origin": "https://abre.ai",
                "Referer": "https://abre.ai/",
                "X-Requested-With": "XMLHttpRequest",
                "User-Agent": random.choice(USER_AGENTS),
                "Accept": "application/json, text/plain, */*",
                "Accept-Language": "pt-BR,pt;q=0.9,en;q=0.8",
            }
            timeout = aiohttp.ClientTimeout(total=TIMEOUT_S)
            async with session.post(
                "https://abre.ai/_/generate",
                data=payload,
                headers=headers,
                timeout=timeout,
                ssl=False,
            ) as resp:
                status = resp.status
                texto = await resp.text()

            if status in (200, 201):
                try:
                    data = json.loads(texto)
                    link = (
                        data.get("data", {})
                            .get("attributes", {})
                            .get("shortenedUrl")
                    )
                    if link and "abre.ai/" in link:
                        # ✅ SUCESSO → grava em links_ok.txt imediatamente
                        async with lock:
                            gerados += 1
                            salvar_linha(ARQUIVO_OK, link)
                        return True
                except (json.JSONDecodeError, KeyError, AttributeError):
                    pass

            elif status == 429:
                # Rate limit → aguardar e tentar novamente
                await asyncio.sleep(random.uniform(2.0, 5.0) * (tentativa + 1))
                continue
            elif status == 403:
                # Bloqueio temporário por IP → aguardar e tentar novamente
                await asyncio.sleep(random.uniform(1.5, 4.0) * (tentativa + 1))
                continue
            elif status in (500, 502, 503, 504):
                await asyncio.sleep(random.uniform(1.0, 3.0) * (tentativa + 1))
                continue

            if tentativa < MAX_RETRIES:
                await asyncio.sleep(random.uniform(0.3, 1.0))
                continue
            break

        except (aiohttp.ClientError, asyncio.TimeoutError):
            if tentativa < MAX_RETRIES:
                await asyncio.sleep(0.5 * (tentativa + 1))
                continue
            break
        except Exception:
            if tentativa < MAX_RETRIES:
                await asyncio.sleep(0.5)
                continue
            break

    # ❌ FALHA → grava URL original em links_fail.txt imediatamente
    async with lock:
        falhados += 1
        salvar_linha(ARQUIVO_FAIL, url_destino)
    return False

# ─── LOOP DE PROGRESSO ────────────────────────────────────────────────────────
async def exibir_progresso(total: int, inicio: float, done_event: asyncio.Event):
    while not done_event.is_set():
        await asyncio.sleep(0.4)
        processados = gerados + falhados
        pct = (processados / total * 100) if total > 0 else 0
        decorrido = time.time() - inicio
        vel = processados / decorrido if decorrido > 0 else 0
        restante = ((total - processados) / vel) if vel > 0 else 0

        sys.stdout.write(
            f"\r{barra(pct)} {pct:5.1f}% | "
            f"{processados:>8,}/{total:,} | "
            f"{C.GR}✅{gerados:>7,}{C.R} | "
            f"{C.RD}❌{falhados:>6,}{C.R} | "
            f"{vel:6.1f}/s | "
            f"⏱ {fmt_tempo(restante)}   "
        )
        sys.stdout.flush()

# ─── MAIN ASYNC ───────────────────────────────────────────────────────────────
async def run(qtd: int, concorrencia: int, url_fixa: str | None):
    global lock

    lock = asyncio.Lock()
    inicio = time.time()

    print(f"\n{C.CY}{C.B}{'='*80}")
    print(f"⚡  GERADOR ULTRA-FAST  |  abre.ai")
    print(f"{'='*80}{C.R}")
    print(f"{C.BL}[CONFIG]{C.R} Links: {C.B}{qtd:,}{C.R} | Concorrência: {C.B}{concorrencia}{C.R}")
    print(f"{C.GR}[SAÍDA OK  ]{C.R}  {ARQUIVO_OK}")
    print(f"{C.RD}[SAÍDA FAIL]{C.R}  {ARQUIVO_FAIL}\n")

    # Limpar arquivos anteriores
    for arq in (ARQUIVO_OK, ARQUIVO_FAIL):
        if os.path.exists(arq):
            os.remove(arq)

    done_event = asyncio.Event()

    connector = aiohttp.TCPConnector(
        limit=concorrencia + 100,
        limit_per_host=0,
        ssl=False,
        ttl_dns_cache=300,
        enable_cleanup_closed=True,
    )
    async with aiohttp.ClientSession(
        connector=connector,
        cookie_jar=DummyCookieJar(),
    ) as session:

        queue: asyncio.Queue = asyncio.Queue()
        for _ in range(qtd):
            queue.put_nowait(None)

        HARD_TIMEOUT = (TIMEOUT_S + 5) * (MAX_RETRIES + 1) + 10

        async def worker(worker_id: int = 0):
            global falhados
            # Escalonar início dos workers para evitar burst de rate limit
            await asyncio.sleep(worker_id * 0.05)
            while True:
                try:
                    queue.get_nowait()
                except asyncio.QueueEmpty:
                    return
                dest = url_fixa if url_fixa else gerar_url_destino()
                try:
                    await asyncio.wait_for(gerar_link(session, dest), timeout=HARD_TIMEOUT)
                except asyncio.TimeoutError:
                    async with lock:
                        falhados += 1
                        salvar_linha(ARQUIVO_FAIL, dest + "  [TIMEOUT]")
                except Exception:
                    async with lock:
                        falhados += 1

        prog_task = asyncio.create_task(exibir_progresso(qtd, inicio, done_event))
        workers = [asyncio.create_task(worker(i)) for i in range(concorrencia)]
        await asyncio.gather(*workers, return_exceptions=True)

        done_event.set()
        await prog_task

    tempo_total = time.time() - inicio
    taxa = (gerados / qtd * 100) if qtd > 0 else 0
    vel_final = gerados / tempo_total if tempo_total > 0 else 0

    print(f"\n\n{C.CY}{C.B}{'='*80}")
    print(f"RESULTADO FINAL")
    print(f"{'='*80}{C.R}")
    print(f"{C.GR}✅ Gerados:{C.R}          {gerados:,}/{qtd:,}  →  {ARQUIVO_OK}")
    print(f"{C.RD}❌ Falhas:{C.R}            {falhados:,}/{qtd:,}  →  {ARQUIVO_FAIL}")
    print(f"{C.YL}Taxa de sucesso:{C.R}     {taxa:.1f}%")
    print(f"{C.BL}Tempo total:{C.R}         {fmt_tempo(tempo_total)} ({tempo_total/60:.2f} min)")
    print(f"{C.MG}Velocidade média:{C.R}    {vel_final:.1f} links/seg")
    print(f"{C.CY}{C.B}{'='*80}{C.R}\n")


def main():
    if os.name == "nt":
        os.system("color")
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())

    print(f"\n{C.CY}{C.B}GERADOR ULTRA-FAST - abre.ai{C.R}")
    print(f"{C.GR}✅ Bons  → {ARQUIVO_OK}{C.R}")
    print(f"{C.RD}❌ Ruins → {ARQUIVO_FAIL}{C.R}\n")

    # ─── URL de destino ───────────────────────────────────────────────────────
    print(f"{C.BL}[?]{C.R} URL de destino:")
    print(f"    {C.YL}1{C.R} - Usar TEMPLATE aleatório")
    print(f"    {C.YL}2{C.R} - Digitar URL fixa")
    while True:
        try:
            opcao = input(f"{C.BL}[?]{C.R} Escolha (1 ou 2): ").strip()
            if opcao == "1":
                url_fixa = None
                break
            elif opcao == "2":
                url_fixa = input(f"{C.BL}[?]{C.R} Digite a URL: ").strip()
                if url_fixa.startswith("http"):
                    break
                print(f"{C.RD}[!]{C.R} URL inválida")
            else:
                print(f"{C.RD}[!]{C.R} Digite 1 ou 2")
        except ValueError:
            print(f"{C.RD}[!]{C.R} Valor inválido")

    # ─── Quantidade ───────────────────────────────────────────────────────────
    while True:
        try:
            qtd = int(input(f"{C.BL}[?]{C.R} Quantidade de links: "))
            if qtd > 0:
                break
            print(f"{C.RD}[!]{C.R} Deve ser maior que 0")
        except ValueError:
            print(f"{C.RD}[!]{C.R} Valor inválido")

    # ─── Concorrência ─────────────────────────────────────────────────────────
    while True:
        try:
            conc = int(input(f"{C.BL}[?]{C.R} Concorrência (padrão {CONCORRENCIA}): ") or str(CONCORRENCIA))
            if conc > 0:
                break
            print(f"{C.RD}[!]{C.R} Valor inválido")
        except ValueError:
            print(f"{C.RD}[!]{C.R} Valor inválido")

    try:
        asyncio.run(run(qtd, conc, url_fixa))
    except KeyboardInterrupt:
        print(f"\n\n{C.RD}[!]{C.R} Interrompido pelo usuário\n")
        sys.exit(0)


if __name__ == "__main__":
    main()

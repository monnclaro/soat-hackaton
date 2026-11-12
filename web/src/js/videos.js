// Lista de vídeos, contadores e atualização automática (polling adaptativo).
import { $, el, formatBytes, timeAgo } from "./dom.js";
import { api } from "./api.js";
import { toast } from "./toast.js";

const LABELS = {
  PENDING: "Na fila",
  PROCESSING: "Processando",
  COMPLETED: "Concluído",
  FAILED: "Falhou",
};

const FAST_POLL_MS = 3000; // há vídeos na fila / processando
const SLOW_POLL_MS = 20000; // nada em andamento

let timer = null;
let running = false;
let lastStatus = new Map(); // id → status, para avisar quando um vídeo termina

function renderStats(videos) {
  const count = (pred) => videos.filter(pred).length;
  $("#statTotal").textContent = videos.length;
  $("#statActive").textContent = count((v) => v.status === "PENDING" || v.status === "PROCESSING");
  $("#statDone").textContent = count((v) => v.status === "COMPLETED");
  $("#statFailed").textContent = count((v) => v.status === "FAILED");
}

function downloadButton(video) {
  const btn = el("button", { class: "btn btn--primary btn--sm video__action", type: "button", text: "Baixar ZIP" });
  btn.addEventListener("click", async () => {
    btn.disabled = true;
    btn.textContent = "Baixando...";
    try {
      await api.downloadZip(video);
    } catch (err) {
      toast(err.message, "err");
    } finally {
      btn.disabled = false;
      btn.textContent = "Baixar ZIP";
    }
  });
  return btn;
}

function renderVideo(v) {
  const meta = el(
    "div",
    { class: "video__meta" },
    el("span", { text: formatBytes(v.size_bytes) }),
    v.frame_count ? el("span", { text: `${v.frame_count} frames` }) : null,
    el("time", { datetime: v.created_at, title: new Date(v.created_at).toLocaleString("pt-BR"), text: `enviado ${timeAgo(v.created_at)}` }),
  );

  return el(
    "article",
    { class: "video", dataset: { status: v.status } },
    el("div", { class: "video__main" }, el("div", { class: "video__name", title: v.original_name, text: v.original_name }), meta),
    el("span", { class: `badge badge--${v.status}`, text: LABELS[v.status] ?? v.status }),
    v.download_url ? downloadButton(v) : null,
    v.error ? el("div", { class: "video__error", text: v.error }) : null,
  );
}

function notifyTransitions(videos) {
  for (const v of videos) {
    const before = lastStatus.get(v.id);
    if (before && before !== v.status) {
      if (v.status === "COMPLETED") toast(`"${v.original_name}" processado: ${v.frame_count} frames.`, "ok");
      if (v.status === "FAILED") toast(`Falha ao processar "${v.original_name}". Você receberá um e-mail.`, "err", 6000);
    }
  }
  lastStatus = new Map(videos.map((v) => [v.id, v.status]));
}

async function refresh() {
  try {
    const videos = await api.listVideos();
    renderStats(videos);
    notifyTransitions(videos);
    const list = $("#videoList");
    list.replaceChildren(
      ...(videos.length
        ? videos.map(renderVideo)
        : [el("p", { class: "empty", text: "Nenhum vídeo enviado ainda. Envie o primeiro acima!" })]),
    );
    return videos.some((v) => v.status === "PENDING" || v.status === "PROCESSING");
  } catch (err) {
    if (err.status !== 401) toast(`Não foi possível carregar os vídeos: ${err.message}`, "err");
    return false;
  }
}

async function tick() {
  if (!running) return;
  clearTimeout(timer);
  const active = await refresh();
  $("#liveIndicator").hidden = !active;
  if (running) timer = setTimeout(tick, active ? FAST_POLL_MS : SLOW_POLL_MS);
}

export const videos = {
  start() {
    running = true;
    lastStatus = new Map();
    tick();
  },
  stop() {
    running = false;
    clearTimeout(timer);
  },
  /** Atualiza agora (após um upload ou clique em "Atualizar"). */
  refresh: () => tick(),
};

export function initVideos() {
  $("#refreshBtn").addEventListener("click", () => videos.refresh());
  // Economiza requisições com a aba em segundo plano.
  document.addEventListener("visibilitychange", () => {
    if (!running) return;
    if (document.hidden) clearTimeout(timer);
    else tick();
  });
}

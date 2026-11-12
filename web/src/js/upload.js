// Envio de vídeos: arrastar e soltar ou seleção múltipla, uploads em paralelo com barra de progresso.
import { $, el, formatBytes } from "./dom.js";
import { api } from "./api.js";
import { toast } from "./toast.js";

const ALLOWED = [".mp4", ".avi", ".mov", ".mkv", ".webm", ".wmv", ".flv", ".m4v"];
const MAX_BYTES = 1024 ** 3; // espelha MAX_UPLOAD_BYTES do video-service

function extension(name) {
  const i = name.lastIndexOf(".");
  return i >= 0 ? name.slice(i).toLowerCase() : "";
}

function validate(file) {
  if (!ALLOWED.includes(extension(file.name))) return "formato não suportado";
  if (file.size === 0) return "arquivo vazio";
  if (file.size > MAX_BYTES) return "maior que 1 GB";
  return "";
}

function createRow(file) {
  const bar = el("div", { class: "progress__bar" });
  const status = el("span", { class: "upload__status", text: "Aguardando..." });
  const row = el(
    "li",
    { class: "upload" },
    el("span", { class: "upload__name", title: file.name, text: `${file.name} · ${formatBytes(file.size)}` }),
    status,
    el("div", { class: "progress", role: "progressbar", "aria-label": `Envio de ${file.name}` }, bar),
  );
  return {
    row,
    progress(fraction) {
      const pct = Math.round(fraction * 100);
      bar.style.width = `${pct}%`;
      status.textContent = pct < 100 ? `Enviando ${pct}%` : "Finalizando...";
    },
    done(message) {
      row.classList.add("is-done");
      bar.style.width = "100%";
      status.textContent = message;
    },
    fail(message) {
      row.classList.add("is-error");
      bar.style.width = "100%";
      status.textContent = message;
    },
  };
}

async function sendAll(files, onUploaded) {
  const list = $("#uploadList");
  const jobs = files.map(async (file) => {
    const ui = createRow(file);
    list.prepend(ui.row);
    const problem = validate(file);
    if (problem) {
      ui.fail(`Recusado: ${problem}`);
      return false;
    }
    try {
      await api.uploadVideo(file, ui.progress);
      ui.done("Enviado ✓ (na fila de processamento)");
      onUploaded();
      return true;
    } catch (err) {
      ui.fail(`Erro: ${err.message}`);
      return false;
    }
  });

  const results = await Promise.all(jobs);
  const ok = results.filter(Boolean).length;
  if (ok) toast(`${ok} vídeo(s) enviados para processamento.`, "ok");
  if (ok < results.length) toast(`${results.length - ok} arquivo(s) não foram enviados.`, "err");

  // Remove as linhas concluídas depois de alguns segundos (os erros ficam visíveis).
  setTimeout(() => list.querySelectorAll(".upload.is-done").forEach((r) => r.remove()), 6000);
}

/** @param {() => void} onUploaded chamado a cada vídeo aceito (atualiza a lista) */
export function initUpload(onUploaded) {
  const zone = $("#dropzone");
  const input = $("#fileInput");

  input.addEventListener("change", () => {
    if (input.files.length) sendAll([...input.files], onUploaded);
    input.value = "";
  });

  zone.addEventListener("dragover", (e) => {
    e.preventDefault();
    zone.classList.add("is-dragging");
  });
  zone.addEventListener("dragleave", () => zone.classList.remove("is-dragging"));
  zone.addEventListener("drop", (e) => {
    e.preventDefault();
    zone.classList.remove("is-dragging");
    const files = [...(e.dataTransfer?.files ?? [])];
    if (files.length) sendAll(files, onUploaded);
  });
}

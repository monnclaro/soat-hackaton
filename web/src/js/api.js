// Cliente da API (via gateway). Contrato em docs/openapi.yaml.
import { session } from "./session.js";

export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

/** Disparado quando o token expira ou é recusado (401). */
export const SESSION_EXPIRED = "fiapx:session-expired";

function authHeaders() {
  return session.token ? { Authorization: `Bearer ${session.token}` } : {};
}

function handleUnauthorized(status) {
  if (status === 401 && session.isLoggedIn) {
    window.dispatchEvent(new CustomEvent(SESSION_EXPIRED));
  }
}

async function request(path, { method = "GET", body } = {}) {
  const headers = { ...authHeaders() };
  if (body !== undefined) headers["Content-Type"] = "application/json";

  let res;
  try {
    res = await fetch(path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
  } catch {
    throw new ApiError(0, "Não foi possível conectar ao servidor.");
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    handleUnauthorized(res.status);
    throw new ApiError(res.status, data.error || `Erro ${res.status}`);
  }
  return data;
}

export const api = {
  register: (name, email, password) => request("/auth/register", { method: "POST", body: { name, email, password } }),
  login: (email, password) => request("/auth/login", { method: "POST", body: { email, password } }),
  listVideos: () => request("/api/videos").then((d) => d.videos ?? []),

  /**
   * Envia um vídeo com acompanhamento de progresso (XHR: fetch ainda não expõe progresso de upload).
   * @param {File} file
   * @param {(fraction: number) => void} onProgress
   */
  uploadVideo(file, onProgress) {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/videos");
      if (session.token) xhr.setRequestHeader("Authorization", `Bearer ${session.token}`);
      xhr.responseType = "json";
      xhr.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) onProgress(e.loaded / e.total);
      });
      xhr.addEventListener("load", () => {
        if (xhr.status >= 200 && xhr.status < 300) return resolve(xhr.response);
        handleUnauthorized(xhr.status);
        reject(new ApiError(xhr.status, xhr.response?.error || `Erro ${xhr.status}`));
      });
      xhr.addEventListener("error", () => reject(new ApiError(0, "Falha de conexão durante o envio.")));
      const form = new FormData();
      form.append("video", file);
      xhr.send(form);
    });
  },

  /** Baixa o ZIP de frames e dispara o "salvar arquivo" do navegador. */
  async downloadZip(video) {
    const res = await fetch(`/api/videos/${encodeURIComponent(video.id)}/download`, { headers: authHeaders() });
    if (!res.ok) {
      handleUnauthorized(res.status);
      const data = await res.json().catch(() => ({}));
      throw new ApiError(res.status, data.error || "Falha no download.");
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = Object.assign(document.createElement("a"), {
      href: url,
      download: video.original_name.replace(/\.[^.]+$/, "") + "_frames.zip",
    });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  },
};

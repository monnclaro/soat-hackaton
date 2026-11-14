// Cliente HTTP da API (via gateway). Contrato completo em docs/openapi.yaml.
import { zipFileName } from "../lib/files";
import type { LoginResponse, User, Video } from "./types";

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** Token ausente, expirado ou recusado: a sessão deve ser encerrada. */
export const isUnauthorized = (err: unknown): boolean => err instanceof ApiError && err.status === 401;

function authHeader(token?: string): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function request<T>(path: string, init: { method?: string; body?: unknown; token?: string } = {}): Promise<T> {
  const headers: Record<string, string> = authHeader(init.token);
  if (init.body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(path, {
      method: init.method ?? "GET",
      headers,
      body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
    });
  } catch {
    throw new ApiError(0, "Não foi possível conectar ao servidor.");
  }
  const data = (await res.json().catch(() => ({}))) as { error?: string };
  if (!res.ok) throw new ApiError(res.status, data.error ?? `Erro ${res.status}`);
  return data as T;
}

export const api = {
  register(name: string, email: string, password: string): Promise<User> {
    return request<User>("/auth/register", { method: "POST", body: { name, email, password } });
  },

  login(email: string, password: string): Promise<LoginResponse> {
    return request<LoginResponse>("/auth/login", { method: "POST", body: { email, password } });
  },

  async listVideos(token: string): Promise<Video[]> {
    const data = await request<{ videos?: Video[] }>("/api/videos", { token });
    return data.videos ?? [];
  },

  /**
   * Envia um vídeo acompanhando o progresso. Usa XMLHttpRequest porque fetch
   * ainda não expõe o progresso do upload.
   */
  uploadVideo(token: string, file: File, onProgress: (fraction: number) => void): Promise<Video> {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/videos");
      xhr.setRequestHeader("Authorization", `Bearer ${token}`);
      xhr.responseType = "json";
      xhr.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) onProgress(e.loaded / e.total);
      });
      xhr.addEventListener("load", () => {
        if (xhr.status >= 200 && xhr.status < 300) resolve(xhr.response as Video);
        else reject(new ApiError(xhr.status, (xhr.response as { error?: string } | null)?.error ?? `Erro ${xhr.status}`));
      });
      xhr.addEventListener("error", () => reject(new ApiError(0, "Falha de conexão durante o envio.")));
      const form = new FormData();
      form.append("video", file);
      xhr.send(form);
    });
  },

  /** Baixa o ZIP de frames e abre o "salvar arquivo" do navegador. */
  async downloadZip(token: string, video: Pick<Video, "id" | "original_name">): Promise<void> {
    const res = await fetch(`/api/videos/${encodeURIComponent(video.id)}/download`, { headers: authHeader(token) });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { error?: string };
      throw new ApiError(res.status, data.error ?? "Falha no download.");
    }
    const url = URL.createObjectURL(await res.blob());
    const a = Object.assign(document.createElement("a"), { href: url, download: zipFileName(video.original_name) });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  },
};

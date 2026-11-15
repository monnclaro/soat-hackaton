// Chamadas ao video-service (/api/videos...). Todas exigem o token do usuário.
import { ApiError, authHeader, readError, request } from "@/shared/api/http";
import { zipFileName } from "./lib/files";
import type { Video } from "./types";

export const videosApi = {
  async list(token: string): Promise<Video[]> {
    const data = await request<{ videos?: Video[] }>("/api/videos", { token });
    return data.videos ?? [];
  },

  /**
   * Envia um vídeo acompanhando o progresso. Usa XMLHttpRequest porque fetch
   * ainda não expõe o progresso do upload.
   */
  upload(token: string, file: File, onProgress: (fraction: number) => void): Promise<Video> {
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
    if (!res.ok) throw await readError(res, "Falha no download.");
    const url = URL.createObjectURL(await res.blob());
    const a = Object.assign(document.createElement("a"), { href: url, download: zipFileName(video.original_name) });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  },
};

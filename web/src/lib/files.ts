// Validação de arquivos antes do envio (espelha as regras do video-service).

export const ALLOWED_EXTENSIONS = [".mp4", ".avi", ".mov", ".mkv", ".webm", ".wmv", ".flv", ".m4v"];
export const MAX_UPLOAD_BYTES = 1024 ** 3; // MAX_UPLOAD_BYTES padrão do video-service (1 GiB)

export function extensionOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i >= 0 ? name.slice(i).toLowerCase() : "";
}

/** Retorna o motivo da recusa ou null se o arquivo pode ser enviado. */
export function validateVideoFile(file: Pick<File, "name" | "size">): string | null {
  if (!ALLOWED_EXTENSIONS.includes(extensionOf(file.name))) return "formato não suportado";
  if (file.size === 0) return "arquivo vazio";
  if (file.size > MAX_UPLOAD_BYTES) return "maior que 1 GB";
  return null;
}

/** "aula.mp4" → "aula_frames.zip" (mesmo nome usado pelo backend). */
export function zipFileName(originalName: string): string {
  const base = originalName.replace(/\.[^.]+$/, "");
  return `${base || "video"}_frames.zip`;
}

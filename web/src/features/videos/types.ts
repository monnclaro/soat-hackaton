// Tipos da API de vídeos (espelham docs/openapi.yaml).

export type VideoStatus = "PENDING" | "PROCESSING" | "COMPLETED" | "FAILED";

export interface Video {
  id: string;
  original_name: string;
  status: VideoStatus;
  size_bytes: number;
  frame_count: number;
  error?: string;
  download_url?: string;
  created_at: string;
  updated_at: string;
}

/** Status em que o vídeo ainda está andando (na fila ou processando). */
export const ACTIVE_STATUSES: readonly VideoStatus[] = ["PENDING", "PROCESSING"];

export const isActive = (v: Pick<Video, "status">): boolean => ACTIVE_STATUSES.includes(v.status);

export const STATUS_LABELS: Record<VideoStatus, string> = {
  PENDING: "Na fila",
  PROCESSING: "Processando",
  COMPLETED: "Concluído",
  FAILED: "Falhou",
};

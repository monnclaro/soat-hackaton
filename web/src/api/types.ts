// Tipos da API (espelham docs/openapi.yaml).

export type VideoStatus = "PENDING" | "PROCESSING" | "COMPLETED" | "FAILED";

export interface User {
  id: string;
  name: string;
  email: string;
  created_at: string;
}

export interface LoginResponse {
  access_token: string;
  token_type: "Bearer";
  expires_at: string;
  user: User;
}

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

export const ACTIVE_STATUSES: readonly VideoStatus[] = ["PENDING", "PROCESSING"];

export const STATUS_LABELS: Record<VideoStatus, string> = {
  PENDING: "Na fila",
  PROCESSING: "Processando",
  COMPLETED: "Concluído",
  FAILED: "Falhou",
};

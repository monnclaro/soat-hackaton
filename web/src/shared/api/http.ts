// Cliente HTTP genérico usado pelas features (chamadas via gateway: /auth e /api).

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

/** Mensagem legível de qualquer erro. */
export const errorMessage = (err: unknown): string => (err instanceof Error ? err.message : String(err));

export function authHeader(token?: string): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Lê a mensagem de erro do corpo ({"error": "..."}) ou usa um texto padrão. */
export async function readError(res: Response, fallback: string): Promise<ApiError> {
  const data = (await res.json().catch(() => ({}))) as { error?: string };
  return new ApiError(res.status, data.error ?? fallback);
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  token?: string;
}

/** Faz a requisição JSON e lança ApiError quando a resposta não é 2xx. */
export async function request<T>(path: string, { method = "GET", body, token }: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = authHeader(token);
  if (body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
  } catch {
    throw new ApiError(0, "Não foi possível conectar ao servidor.");
  }
  if (!res.ok) throw await readError(res, `Erro ${res.status}`);
  return (await res.json().catch(() => ({}))) as T;
}

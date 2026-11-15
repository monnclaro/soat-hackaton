// Simulações de fetch e XMLHttpRequest para os testes de componentes.
import { vi } from "vitest";
import type { Video } from "@/features/videos";

export type Route = (init?: RequestInit) => { status: number; body?: unknown };

/** Substitui o fetch global por rotas "MÉTODO /caminho". Retorna o mock para inspeção. */
export function mockFetch(routes: Record<string, Route>) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${String(input)}`;
    const route = routes[key];
    if (!route) throw new Error(`rota não simulada: ${key}`);
    const { status, body } = route(init);
    return new Response(body === undefined ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

export function video(partial: Partial<Video> = {}): Video {
  return {
    id: "v1",
    original_name: "aula.mp4",
    status: "PENDING",
    size_bytes: 2048,
    frame_count: 0,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...partial,
  };
}

export const user = { id: "u1", name: "Ana Souza", email: "ana@fiapx.com", created_at: "2026-11-14T12:00:00Z" };

/** XMLHttpRequest falso: registra os envios e responde com o status configurado. */
export class FakeXHR {
  static instances: FakeXHR[] = [];
  static respondWith = { status: 202, body: video() as unknown };

  upload = new EventTarget();
  status = 0;
  response: unknown = null;
  responseType = "";
  method = "";
  url = "";
  headers: Record<string, string> = {};
  private listeners: Record<string, () => void> = {};

  constructor() {
    FakeXHR.instances.push(this);
  }
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v;
  }
  addEventListener(type: string, fn: () => void) {
    this.listeners[type] = fn;
  }
  send() {
    queueMicrotask(() => {
      const progress = Object.assign(new Event("progress"), { lengthComputable: true, loaded: 50, total: 100 });
      this.upload.dispatchEvent(progress);
      this.status = FakeXHR.respondWith.status;
      this.response = FakeXHR.respondWith.body;
      this.listeners.load?.();
    });
  }
}

export function installFakeXHR() {
  FakeXHR.instances = [];
  FakeXHR.respondWith = { status: 202, body: video() };
  vi.stubGlobal("XMLHttpRequest", FakeXHR);
}

// Avisos flutuantes (canto inferior direito), disponíveis para toda a aplicação.
import { createContext, useCallback, useContext, useState, type ReactNode } from "react";

export type ToastTone = "ok" | "info" | "err";

interface Toast {
  id: number;
  message: string;
  tone: ToastTone;
}

export type Notify = (message: string, tone?: ToastTone, durationMs?: number) => void;

const ToastContext = createContext<Notify | null>(null);

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const notify = useCallback<Notify>((message, tone = "info", durationMs = 4000) => {
    const id = nextId++;
    setToasts((list) => [...list, { id, message, tone }]);
    setTimeout(() => setToasts((list) => list.filter((t) => t.id !== id)), durationMs);
  }, []);

  return (
    <ToastContext.Provider value={notify}>
      {children}
      <div className="toasts" aria-live="assertive">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast--${t.tone}`} role="status">
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): Notify {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast deve ser usado dentro de <ToastProvider>");
  return ctx;
}

// Sessão do usuário (token JWT + dados básicos), persistida no localStorage.
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import type { User } from "./types";

export interface Session {
  token: string;
  user: User;
}

interface SessionValue {
  session: Session | null;
  start: (session: Session) => void;
  end: () => void;
}

const STORAGE_KEY = "fiapx_session";

function load(): Session | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    return null; // storage bloqueado (modo privado) ou valor corrompido
  }
}

function save(session: Session | null) {
  try {
    if (session) localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    // sem storage: a sessão dura só até recarregar a página
  }
}

const SessionContext = createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(load);

  const start = useCallback((s: Session) => {
    save(s);
    setSession(s);
  }, []);

  const end = useCallback(() => {
    save(null);
    setSession(null);
  }, []);

  const value = useMemo(() => ({ session, start, end }), [session, start, end]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession deve ser usado dentro de <SessionProvider>");
  return ctx;
}

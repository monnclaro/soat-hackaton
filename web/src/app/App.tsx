// Raiz da aplicação: providers globais + escolha entre a tela de acesso e a área logada.
import { AuthPage, SessionProvider, useSession } from "@/features/auth";
import { Dashboard } from "@/features/videos";
import { ToastProvider } from "@/shared/toast/ToastProvider";
import { Topbar } from "./Topbar";

function Shell() {
  const { session } = useSession();
  return (
    <>
      <Topbar />
      <main className="container">
        {session ? <Dashboard key={session.token} session={session} /> : <AuthPage />}
      </main>
    </>
  );
}

export function App() {
  return (
    <SessionProvider>
      <ToastProvider>
        <Shell />
      </ToastProvider>
    </SessionProvider>
  );
}

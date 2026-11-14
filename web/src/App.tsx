import { AuthPage } from "./components/AuthPage";
import { Dashboard } from "./components/Dashboard";
import { Topbar } from "./components/Topbar";
import { SessionProvider, useSession } from "./session/SessionContext";
import { ToastProvider } from "./toast/ToastContext";

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

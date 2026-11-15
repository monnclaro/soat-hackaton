import { useSession } from "@/features/auth";
import { initials } from "@/shared/lib/format";

export function Topbar() {
  const { session, end } = useSession();
  return (
    <header className="topbar">
      <a className="brand" href="/" aria-label="FIAP X - início">
        <span className="brand__logo" aria-hidden="true">
          ▶
        </span>
        <span>
          FIAP <strong>X</strong>
        </span>
        <span className="brand__subtitle">Processador de Vídeos</span>
      </a>
      {session && (
        <div className="topbar__user">
          <span className="avatar" aria-hidden="true">
            {initials(session.user.name)}
          </span>
          <span className="topbar__name">{session.user.name}</span>
          <button className="btn btn--ghost btn--sm" type="button" onClick={end}>
            Sair
          </button>
        </div>
      )}
    </header>
  );
}

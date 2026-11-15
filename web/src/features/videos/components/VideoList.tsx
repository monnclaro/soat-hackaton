import type { Video } from "../types";
import { VideoCard } from "./VideoCard";

interface Props {
  token: string;
  videos: Video[];
  loaded: boolean;
  active: boolean;
  onRefresh: () => void;
  onUnauthorized: () => void;
}

/** Card "Meus vídeos": indicador de atualização, botão Atualizar e a lista. */
export function VideoList({ token, videos, loaded, active, onRefresh, onUnauthorized }: Props) {
  return (
    <div className="card">
      <div className="card__header">
        <h2 className="card__title">Meus vídeos</h2>
        <div className="card__actions">
          {active && (
            <span className="live">
              <span className="live__dot" />
              atualizando
            </span>
          )}
          <button className="btn btn--ghost btn--sm" type="button" onClick={onRefresh}>
            Atualizar
          </button>
        </div>
      </div>
      <div className="videos" aria-live="polite">
        {!loaded && <p className="empty">Carregando...</p>}
        {loaded && videos.length === 0 && <p className="empty">Nenhum vídeo enviado ainda. Envie o primeiro acima!</p>}
        {videos.map((v) => (
          <VideoCard key={v.id} token={token} video={v} onUnauthorized={onUnauthorized} />
        ))}
      </div>
    </div>
  );
}

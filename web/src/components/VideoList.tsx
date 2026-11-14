import { useState } from "react";
import { api, isUnauthorized } from "../api/client";
import { STATUS_LABELS, type Video } from "../api/types";
import { formatBytes, timeAgo } from "../lib/format";
import { useToast } from "../toast/ToastContext";

interface Props {
  token: string;
  videos: Video[];
  loaded: boolean;
  active: boolean;
  onRefresh: () => void;
  onUnauthorized: () => void;
}

function DownloadButton({ token, video, onUnauthorized }: { token: string; video: Video; onUnauthorized: () => void }) {
  const [busy, setBusy] = useState(false);
  const notify = useToast();

  async function download() {
    setBusy(true);
    try {
      await api.downloadZip(token, video);
    } catch (err) {
      if (isUnauthorized(err)) onUnauthorized();
      else notify(err instanceof Error ? err.message : String(err), "err");
    } finally {
      setBusy(false);
    }
  }

  return (
    <button className="btn btn--primary btn--sm video__action" type="button" disabled={busy} onClick={download}>
      {busy ? "Baixando..." : "Baixar ZIP"}
    </button>
  );
}

export function VideoCard({ token, video, onUnauthorized }: { token: string; video: Video; onUnauthorized: () => void }) {
  return (
    <article className="video" data-status={video.status}>
      <div className="video__main">
        <div className="video__name" title={video.original_name}>
          {video.original_name}
        </div>
        <div className="video__meta">
          <span>{formatBytes(video.size_bytes)}</span>
          {video.frame_count > 0 && <span>{video.frame_count} frames</span>}
          <time dateTime={video.created_at} title={new Date(video.created_at).toLocaleString("pt-BR")}>
            enviado {timeAgo(video.created_at)}
          </time>
        </div>
      </div>
      <span className={`badge badge--${video.status}`}>{STATUS_LABELS[video.status] ?? video.status}</span>
      {video.download_url && <DownloadButton token={token} video={video} onUnauthorized={onUnauthorized} />}
      {video.error && <div className="video__error">{video.error}</div>}
    </article>
  );
}

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

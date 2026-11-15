import { useState } from "react";
import { errorMessage, isUnauthorized } from "@/shared/api/http";
import { formatBytes, timeAgo } from "@/shared/lib/format";
import { useToast } from "@/shared/toast/ToastProvider";
import { videosApi } from "../api";
import { STATUS_LABELS, type Video } from "../types";

interface Props {
  token: string;
  video: Video;
  onUnauthorized: () => void;
}

function DownloadButton({ token, video, onUnauthorized }: Props) {
  const [busy, setBusy] = useState(false);
  const notify = useToast();

  async function download() {
    setBusy(true);
    try {
      await videosApi.downloadZip(token, video);
    } catch (err) {
      if (isUnauthorized(err)) onUnauthorized();
      else notify(errorMessage(err), "err");
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

/** Um vídeo da lista: nome, tamanho, frames, status, download e erro. */
export function VideoCard(props: Props) {
  const { video } = props;
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
      {video.download_url && <DownloadButton {...props} />}
      {video.error && <div className="video__error">{video.error}</div>}
    </article>
  );
}

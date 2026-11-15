// Área logada: contadores, envio e lista de vídeos.
import { useCallback } from "react";
import { useSession, type Session } from "@/features/auth";
import { useToast } from "@/shared/toast/ToastProvider";
import { useVideos } from "../hooks/useVideos";
import { Stats } from "./Stats";
import { UploadPanel } from "./UploadPanel";
import { VideoList } from "./VideoList";

export function Dashboard({ session }: { session: Session }) {
  const { end } = useSession();
  const notify = useToast();

  const expire = useCallback(() => {
    end();
    notify("Sua sessão expirou. Entre novamente.", "info");
  }, [end, notify]);

  const { videos, loaded, active, refresh } = useVideos(session.token, {
    onUnauthorized: expire,
    onError: (message) => notify(`Não foi possível carregar os vídeos: ${message}`, "err"),
    onTransition: (video) => {
      if (video.status === "COMPLETED") notify(`"${video.original_name}" processado: ${video.frame_count} frames.`, "ok");
      if (video.status === "FAILED") notify(`Falha ao processar "${video.original_name}". Você receberá um e-mail.`, "err", 6000);
    },
  });

  return (
    <section className="app">
      <Stats videos={videos} />
      <UploadPanel token={session.token} onUploaded={refresh} onUnauthorized={expire} />
      <VideoList
        token={session.token}
        videos={videos}
        loaded={loaded}
        active={active}
        onRefresh={refresh}
        onUnauthorized={expire}
      />
    </section>
  );
}

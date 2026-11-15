// Lista os vídeos do usuário com atualização automática adaptativa:
// a cada 3s enquanto houver vídeo na fila/processando, a cada 20s caso contrário,
// e pausada quando a aba fica em segundo plano.
import { useCallback, useEffect, useRef, useState } from "react";
import { errorMessage, isUnauthorized } from "@/shared/api/http";
import { videosApi } from "../api";
import { isActive, type Video, type VideoStatus } from "../types";

export const FAST_POLL_MS = 3000;
export const SLOW_POLL_MS = 20000;

interface Options {
  onUnauthorized: () => void;
  onError?: (message: string) => void;
  /** Chamado quando um vídeo já conhecido muda de status. */
  onTransition?: (video: Video, previous: VideoStatus) => void;
}

export interface VideosState {
  videos: Video[];
  loaded: boolean;
  active: boolean;
  refresh: () => void;
}

export function useVideos(token: string, options: Options): VideosState {
  const [videos, setVideos] = useState<Video[]>([]);
  const [loaded, setLoaded] = useState(false);

  // Refs evitam reiniciar o polling quando os callbacks mudam de identidade.
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const lastStatus = useRef(new Map<string, VideoStatus>());
  const stopped = useRef(false);

  const tick = useCallback(async () => {
    clearTimeout(timer.current);
    let active = false;
    try {
      const list = await videosApi.list(token);
      if (stopped.current) return;
      for (const v of list) {
        const prev = lastStatus.current.get(v.id);
        if (prev && prev !== v.status) optionsRef.current.onTransition?.(v, prev);
      }
      lastStatus.current = new Map(list.map((v) => [v.id, v.status]));
      active = list.some(isActive);
      setVideos(list);
      setLoaded(true);
    } catch (err) {
      if (stopped.current) return;
      if (isUnauthorized(err)) {
        optionsRef.current.onUnauthorized();
        return;
      }
      optionsRef.current.onError?.(errorMessage(err));
    }
    if (!stopped.current && !document.hidden) {
      timer.current = setTimeout(tick, active ? FAST_POLL_MS : SLOW_POLL_MS);
    }
  }, [token]);

  useEffect(() => {
    stopped.current = false;
    lastStatus.current = new Map();
    void tick();

    const onVisibility = () => {
      if (document.hidden) clearTimeout(timer.current);
      else void tick();
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      stopped.current = true;
      clearTimeout(timer.current);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [tick]);

  const refresh = useCallback(() => void tick(), [tick]);
  const active = videos.some(isActive);
  return { videos, loaded, active, refresh };
}

// Envio de vídeos: arrastar e soltar ou seleção múltipla, envios em paralelo com progresso.
import { useRef, useState, type DragEvent } from "react";
import { api, isUnauthorized } from "../api/client";
import { formatBytes } from "../lib/format";
import { validateVideoFile } from "../lib/files";
import { useToast } from "../toast/ToastContext";

type UploadState = "waiting" | "uploading" | "done" | "error";

interface UploadItem {
  id: number;
  name: string;
  size: number;
  progress: number; // 0..1
  state: UploadState;
  message: string;
}

interface Props {
  token: string;
  onUploaded: () => void;
  onUnauthorized: () => void;
}

let nextId = 1;

export function UploadPanel({ token, onUploaded, onUnauthorized }: Props) {
  const [items, setItems] = useState<UploadItem[]>([]);
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const notify = useToast();

  const update = (id: number, patch: Partial<UploadItem>) =>
    setItems((list) => list.map((i) => (i.id === id ? { ...i, ...patch } : i)));

  async function sendOne(file: File, id: number): Promise<boolean> {
    const problem = validateVideoFile(file);
    if (problem) {
      update(id, { state: "error", progress: 1, message: `Recusado: ${problem}` });
      return false;
    }
    update(id, { state: "uploading", message: "Enviando 0%" });
    try {
      await api.uploadVideo(token, file, (fraction) => {
        const pct = Math.round(fraction * 100);
        update(id, { progress: fraction, message: pct < 100 ? `Enviando ${pct}%` : "Finalizando..." });
      });
      update(id, { state: "done", progress: 1, message: "Enviado ✓ (na fila de processamento)" });
      onUploaded();
      return true;
    } catch (err) {
      if (isUnauthorized(err)) onUnauthorized();
      update(id, { state: "error", progress: 1, message: `Erro: ${err instanceof Error ? err.message : String(err)}` });
      return false;
    }
  }

  async function sendAll(files: File[]) {
    if (!files.length) return;
    const batch = files.map((f) => ({ file: f, id: nextId++ }));
    setItems((list) => [
      ...batch.map(({ file, id }) => ({
        id,
        name: file.name,
        size: file.size,
        progress: 0,
        state: "waiting" as const,
        message: "Aguardando...",
      })),
      ...list,
    ]);

    const results = await Promise.all(batch.map(({ file, id }) => sendOne(file, id)));
    const ok = results.filter(Boolean).length;
    if (ok) notify(`${ok} vídeo(s) enviados para processamento.`, "ok");
    if (ok < results.length) notify(`${results.length - ok} arquivo(s) não foram enviados.`, "err");

    // Remove os concluídos depois de alguns segundos; os erros continuam visíveis.
    const doneIds = new Set(batch.map((b) => b.id));
    setTimeout(() => setItems((list) => list.filter((i) => !(doneIds.has(i.id) && i.state === "done"))), 6000);
  }

  function onDrop(e: DragEvent<HTMLLabelElement>) {
    e.preventDefault();
    setDragging(false);
    void sendAll(Array.from(e.dataTransfer?.files ?? []));
  }

  return (
    <div className="card">
      <h2 className="card__title">Enviar vídeos</h2>
      <label
        className={`dropzone${dragging ? " is-dragging" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
      >
        <input
          ref={inputRef}
          className="dropzone__input"
          type="file"
          multiple
          accept="video/*,.mp4,.avi,.mov,.mkv,.webm,.wmv,.flv,.m4v"
          aria-label="Selecionar vídeos"
          onChange={(e) => {
            void sendAll(Array.from(e.target.files ?? []));
            e.target.value = "";
          }}
        />
        <span className="dropzone__icon" aria-hidden="true">
          ⬆
        </span>
        <span className="dropzone__text">
          <strong>Arraste os vídeos aqui</strong> ou clique para selecionar
        </span>
        <span className="dropzone__hint">mp4, avi, mov, mkv, webm, wmv, flv, m4v · até 1 GB cada</span>
      </label>

      <ul className="uploads" aria-live="polite">
        {items.map((i) => (
          <li key={i.id} className={`upload${i.state === "done" ? " is-done" : ""}${i.state === "error" ? " is-error" : ""}`}>
            <span className="upload__name" title={i.name}>
              {i.name} · {formatBytes(i.size)}
            </span>
            <span className="upload__status">{i.message}</span>
            <div
              className="progress"
              role="progressbar"
              aria-label={`Envio de ${i.name}`}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(i.progress * 100)}
            >
              <div className="progress__bar" style={{ width: `${Math.round(i.progress * 100)}%` }} />
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

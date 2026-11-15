import { isActive, type Video } from "../types";

/** Contadores do topo: total, em andamento, concluídos e com falha. */
export function Stats({ videos }: { videos: Video[] }) {
  const count = (pred: (v: Video) => boolean) => videos.filter(pred).length;
  const items = [
    { label: "Vídeos", value: videos.length, tone: "" },
    { label: "Na fila / processando", value: count(isActive), tone: "info" },
    { label: "Concluídos", value: count((v) => v.status === "COMPLETED"), tone: "ok" },
    { label: "Com falha", value: count((v) => v.status === "FAILED"), tone: "err" },
  ];
  return (
    <div className="stats" aria-label="Resumo dos seus vídeos">
      {items.map((i) => (
        <div key={i.label} className={`stat${i.tone ? ` stat--${i.tone}` : ""}`}>
          <span className="stat__value">{i.value}</span>
          <span className="stat__label">{i.label}</span>
        </div>
      ))}
    </div>
  );
}

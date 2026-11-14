// Formatação de valores para exibição.

export function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** i).toFixed(i ? 1 : 0)} ${units[i]}`;
}

const rtf = new Intl.RelativeTimeFormat("pt-BR", { numeric: "auto" });

/** Tempo relativo em português ("há 5 minutos", "ontem"...). */
export function timeAgo(isoDate: string, now: number = Date.now()): string {
  let value = Math.round((new Date(isoDate).getTime() - now) / 1000);
  const steps: [number, Intl.RelativeTimeFormatUnit][] = [
    [60, "second"],
    [60, "minute"],
    [24, "hour"],
    [30, "day"],
  ];
  for (const [size, unit] of steps) {
    if (Math.abs(value) < size) return rtf.format(value, unit);
    value = Math.round(value / size);
  }
  return new Date(isoDate).toLocaleDateString("pt-BR");
}

/** Iniciais do nome para o avatar ("Ana Souza" → "AS"). */
export function initials(name = ""): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p.charAt(0).toUpperCase())
    .join("");
}

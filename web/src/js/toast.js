// Notificações flutuantes (canto inferior direito).
import { $, el } from "./dom.js";

/** @param {"ok"|"info"|"err"} tone */
export function toast(message, tone = "info", ms = 4000) {
  const node = el("div", { class: `toast toast--${tone}`, role: "status", text: message });
  $("#toasts").append(node);
  setTimeout(() => {
    node.classList.add("is-leaving");
    setTimeout(() => node.remove(), 200);
  }, ms);
}

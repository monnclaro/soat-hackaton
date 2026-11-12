// Ponto de entrada da UI: alterna entre a tela de acesso e a área logada.
import { $, $$, initials } from "./dom.js";
import { SESSION_EXPIRED } from "./api.js";
import { session } from "./session.js";
import { initAuth } from "./auth.js";
import { initUpload } from "./upload.js";
import { initVideos, videos } from "./videos.js";
import { toast } from "./toast.js";

function render() {
  const loggedIn = session.isLoggedIn;
  for (const node of $$("[data-auth]")) {
    node.hidden = node.dataset.auth !== (loggedIn ? "in" : "out");
  }
  if (loggedIn) {
    $("#userName").textContent = session.user?.name ?? "";
    $("#userAvatar").textContent = initials(session.user?.name);
    videos.start();
  } else {
    videos.stop();
    $("#uploadList").replaceChildren();
    $("#loginForm input")?.focus();
  }
}

function logout() {
  session.end();
  render();
}

initAuth(render);
initUpload(() => videos.refresh());
initVideos();

$("#logoutBtn").addEventListener("click", logout);
window.addEventListener(SESSION_EXPIRED, () => {
  logout();
  toast("Sua sessão expirou. Entre novamente.", "info");
});

render();

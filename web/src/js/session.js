// Sessão do usuário (token JWT + dados básicos) persistida no localStorage.

const TOKEN_KEY = "fiapx_token";
const USER_KEY = "fiapx_user";

function read(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key, value) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // modo privado / storage bloqueado: a sessão dura só até recarregar a página
  }
}

let token = read(TOKEN_KEY);
let user = JSON.parse(read(USER_KEY) || "null");

export const session = {
  get token() {
    return token;
  },
  get user() {
    return user;
  },
  get isLoggedIn() {
    return Boolean(token);
  },
  start(accessToken, userData) {
    token = accessToken;
    user = userData;
    write(TOKEN_KEY, token);
    write(USER_KEY, JSON.stringify(user));
  },
  end() {
    token = null;
    user = null;
    write(TOKEN_KEY, null);
    write(USER_KEY, null);
  },
};

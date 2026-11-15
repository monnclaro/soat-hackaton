// API pública da feature de autenticação: o restante da aplicação importa só daqui.
export { AuthPage } from "./components/AuthPage";
export { SessionProvider, useSession, type Session } from "./session";
export type { User } from "./types";

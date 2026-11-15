// Chamadas ao auth-service (POST /auth/register, POST /auth/login).
import { request } from "@/shared/api/http";
import type { LoginResponse, User } from "./types";

export const authApi = {
  register(name: string, email: string, password: string): Promise<User> {
    return request<User>("/auth/register", { method: "POST", body: { name, email, password } });
  },

  login(email: string, password: string): Promise<LoginResponse> {
    return request<LoginResponse>("/auth/login", { method: "POST", body: { email, password } });
  },
};

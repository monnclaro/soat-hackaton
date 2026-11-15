// Tipos da API de identidade (espelham docs/openapi.yaml).

export interface User {
  id: string;
  name: string;
  email: string;
  created_at: string;
}

export interface LoginResponse {
  access_token: string;
  token_type: "Bearer";
  expires_at: string;
  user: User;
}

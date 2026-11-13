// Reuse the official, memory-only refresh lifecycle rather than treating its
// HttpOnly refresh cookie as authorization for management endpoints.
import {
  clearAuthentication,
  getFreshAuthHeaders,
  refreshAuthentication,
  resolveAuthentication,
} from "@/lib/auth-session";
import { sanitizeAuthRedirect } from "@/features/auth/lib/auth-redirect";
import { useAuthStore } from "@/stores/auth-store";

export type SelfUser = {
  id: number;
  username: string;
  display_name?: string;
  role: number;
  group?: string;
};

export class ApiError extends Error {
  status: number;

  constructor(status: number) {
    // Never expose transport errors or server bodies that might contain secrets.
    super(status === 401 ? "Session expired!" : "Request failed");
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(path: "/api/user/self" | "/api/status"): Promise<T> {
  const authenticated = path === "/api/user/self";
  if (authenticated) {
    const outcome = await resolveAuthentication();
    if (outcome.kind !== "authenticated") {
      throw new ApiError(outcome.kind === "anonymous" ? 401 : 503);
    }
  }
  const initialSID = useAuthStore.getState().auth.session?.sid;

  // Only these read-only endpoints can be replayed. Never use this loop for
  // mutations or one-time security proofs.
  for (let attempt = 0; attempt < 2; attempt++) {
    const headers: Record<string, string> = {
      Accept: "application/json",
      "Cache-Control": "no-cache, no-store",
    };
    if (authenticated) {
      try {
        Object.assign(headers, await getFreshAuthHeaders());
      } catch {
        throw new ApiError(503);
      }
      if (useAuthStore.getState().auth.session?.sid !== initialSID) throw new ApiError(409);
    }

    let response: Response;
    try {
      response = await fetch(path, {
        method: "GET",
        credentials: "same-origin",
        mode: "same-origin",
        cache: "no-store",
        redirect: "error",
        headers,
      });
    } catch {
      throw new ApiError(503);
    }
    if (authenticated && useAuthStore.getState().auth.session?.sid !== initialSID)
      throw new ApiError(409);
    if (response.status === 401 && authenticated) {
      if (attempt === 1) {
        clearAuthentication(false);
        throw new ApiError(401);
      }
      const outcome = await refreshAuthentication();
      if (outcome.kind !== "authenticated") {
        throw new ApiError(outcome.kind === "anonymous" ? 401 : 503);
      }
      if (outcome.bundle.session.sid !== initialSID) throw new ApiError(409);
      continue;
    }
    if (!response.ok) throw new ApiError(response.status);
    let body: { success?: boolean; data?: T };
    try {
      body = await response.json();
    } catch {
      throw new ApiError(502);
    }
    if (authenticated && useAuthStore.getState().auth.session?.sid !== initialSID)
      throw new ApiError(409);
    if (body?.success !== true || body.data == null) throw new ApiError(502);
    return body.data;
  }
  throw new ApiError(401);
}

export const api = {
  self: (): Promise<SelfUser> => request<SelfUser>("/api/user/self"),
  status: (): Promise<Record<string, unknown>> => request<Record<string, unknown>>("/api/status"),
};

// Deliberately fixed: no query/hash/user-supplied destination is propagated.
export function officialLoginUrl(): string {
  const target = sanitizeAuthRedirect("/cmh/whoami", window.location.origin);
  if (target !== "/cmh/whoami") return "/sign-in";
  return `/sign-in?redirect=${encodeURIComponent(target)}`;
}

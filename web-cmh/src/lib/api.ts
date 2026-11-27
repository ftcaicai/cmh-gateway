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

async function request<T>(
  path: string,
  method: "GET" | "POST" | "PUT" = "GET",
  input?: unknown,
): Promise<T> {
  const authenticated = path !== "/api/status";
  if (authenticated) {
    const outcome = await resolveAuthentication();
    if (outcome.kind !== "authenticated") {
      throw new ApiError(outcome.kind === "anonymous" ? 401 : 503);
    }
  }
  const initialSID = useAuthStore.getState().auth.session?.sid;

  // Read-only requests may refresh once; mutations are never automatically replayed.
  for (let attempt = 0; attempt < (method === "GET" ? 2 : 1); attempt++) {
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

    if (input !== undefined) headers["Content-Type"] = "application/json";
    let response: Response;
    try {
      response = await fetch(path, {
        method,
        ...(method === "GET" ? {} : { body: JSON.stringify(input) }),
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
      if (attempt === 1 || method !== "GET") {
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

export type BrandInput = { name: string; support_email: string; description: string };
export type BrandSite = BrandInput & {
  id: string;
  version: number;
  created_at: number;
  updated_at: number;
  access: "owner" | "readonly";
};
export type BrandDomain = {
  available: boolean;
  host: string;
  cname_target: string;
  verified: boolean;
  verified_at: number;
  checked_at: number;
  last_error: string;
  // HTTPS readiness of the verified host; all zero until first checked.
  tls_ready: boolean;
  tls_not_after: number;
  tls_checked_at: number;
  tls_error: string;
};
export type BrandPause = { paused: boolean; paused_at: number };
export type LogoHistoryEntry = {
  id: string;
  content_type: string;
  size: number;
  replaced_at: number;
};
export type PreviewPage = "home" | "docs" | "login";
export type BrandAppearanceInput = {
  theme: string;
  seo_title: string;
  seo_description: string;
  seo_keywords: string;
  allow_indexing: boolean;
};
export type BrandAppearance = BrandAppearanceInput & {
  has_logo: boolean;
  logo_updated_at: number;
  version: number;
};
export type BrandAudit = { id: string; action: string; version: number; created_at: number };

export function brandInput(input: BrandInput): BrandInput {
  return { name: input.name, support_email: input.support_email, description: input.description };
}

export type Customer = {
  user_id: number;
  site_id: string;
  active: boolean;
  joined_at: number;
  revoked_at: number;
};
export type CustomerInvitation = {
  id: string;
  site_id: string;
  target_user_id: number;
  expires_at: number;
  created_at: number;
  accepted_at: number;
};
export type CustomerNote = { note: string; version: number; updated_at: number };
export type CustomerAudit = { id: string; action: string; user_id: number; created_at: number };
export type ModelCatalog = {
  models: string[];
  model_policy_enabled: boolean;
  destination_policy_enabled: boolean;
};

export type TeamMember = Customer & { version: number };
export type TeamInvitation = CustomerInvitation;
export type TeamAudit = CustomerAudit;

// Lists are newest first in windows of PAGE_SIZE; the server caps limit at 100.
export const PAGE_SIZE = 50;
export type Page = { offset?: number; limit?: number };
function pageQuery(page: Page): string {
  const params = new URLSearchParams();
  if (page.offset) params.set("offset", String(page.offset));
  if (page.limit) params.set("limit", String(page.limit));
  const query = params.toString();
  return query ? `?${query}` : "";
}

export function ownsBrand(site: BrandSite | null | undefined): boolean {
  return site?.access === "owner";
}

export const api = {
  brands: (): Promise<BrandSite[]> => request("/api/ext/oem/sites"),
  brand: (id: string): Promise<BrandSite> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}`),
  createBrand: (input: BrandInput): Promise<BrandSite> =>
    request("/api/ext/oem/sites", "POST", brandInput(input)),
  updateBrand: (id: string, version: number, input: BrandInput): Promise<BrandSite> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}`, "PUT", {
      ...brandInput(input),
      version,
    }),
  brandDomain: (id: string): Promise<BrandDomain> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/domain`),
  setBrandDomain: (id: string, host: string): Promise<BrandDomain> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/domain`, "PUT", { host }),
  verifyBrandDomain: (id: string): Promise<BrandDomain> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/domain/verify`, "POST", {}),
  brandAppearance: (id: string): Promise<BrandAppearance> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/appearance`),
  updateBrandAppearance: (
    id: string,
    input: BrandAppearanceInput & { version: number },
  ): Promise<BrandAppearance> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/appearance`, "PUT", {
      theme: input.theme,
      seo_title: input.seo_title,
      seo_description: input.seo_description,
      seo_keywords: input.seo_keywords,
      allow_indexing: input.allow_indexing,
      version: input.version,
    }),
  brandLogo: (id: string): Promise<{ content_type: string; image: string }> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/logo`),
  setBrandLogo: (id: string, image: string): Promise<BrandAppearance> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/logo`, "PUT", { image }),
  logoHistory: (id: string): Promise<LogoHistoryEntry[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/logo-history`),
  historicLogo: (id: string, entry: string): Promise<{ content_type: string; image: string }> =>
    request(
      `/api/ext/oem/sites/${encodeURIComponent(id)}/logo-history/${encodeURIComponent(entry)}`,
    ),
  restoreLogo: (id: string, entry: string): Promise<BrandAppearance> =>
    request(
      `/api/ext/oem/sites/${encodeURIComponent(id)}/logo-history/${encodeURIComponent(entry)}/restore`,
      "POST",
      {},
    ),
  brandPreview: (id: string, theme: string, page: PreviewPage): Promise<{ html: string }> =>
    request(
      `/api/ext/oem/sites/${encodeURIComponent(id)}/preview?${new URLSearchParams({ theme, page })}`,
    ),
  brandPause: (id: string): Promise<BrandPause> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/pause`),
  setBrandPause: (id: string, paused: boolean): Promise<BrandPause> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/pause`, "PUT", { paused }),
  brandAudits: (id: string, page: Page = {}): Promise<BrandAudit[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(id)}/audits${pageQuery(page)}`),
  customerAudits: (site: string, page: Page = {}): Promise<CustomerAudit[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customer-audits${pageQuery(page)}`),
  customers: (site: string, page: Page = {}): Promise<Customer[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customers${pageQuery(page)}`),
  customerNote: (site: string, user: number): Promise<CustomerNote> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customers/${user}/notes`),
  updateCustomerNote: (
    site: string,
    user: number,
    version: number,
    note: string,
  ): Promise<CustomerNote> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customers/${user}/notes`, "PUT", {
      note,
      version,
    }),
  invitations: (site: string): Promise<CustomerInvitation[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customer-invitations`),
  inviteCustomer: (
    site: string,
    targetUserID: number,
  ): Promise<{ invitation: CustomerInvitation; token: string }> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customer-invitations`, "POST", {
      target_user_id: targetUserID,
    }),
  revokeCustomer: (site: string, user: number): Promise<Customer> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/customers/${user}/revoke`, "POST", {}),
  membership: (): Promise<{ membership: Customer | null }> =>
    request("/api/ext/oem/customer-membership"),
  acceptInvitation: (token: string): Promise<Customer> =>
    request("/api/ext/oem/customer-invitations/accept", "POST", { token }),
  team: (site: string): Promise<TeamMember[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/team`),
  teamInvitations: (site: string): Promise<TeamInvitation[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/team-invitations`),
  teamAudits: (site: string, page: Page = {}): Promise<TeamAudit[]> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/team-audits${pageQuery(page)}`),
  inviteTeamMember: (
    site: string,
    user: number,
  ): Promise<{ invitation: TeamInvitation; token: string }> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/team-invitations`, "POST", {
      target_user_id: user,
    }),
  acceptTeamInvitation: (token: string): Promise<TeamMember> =>
    request("/api/ext/oem/team-invitations/accept", "POST", { token }),
  revokeTeamMember: (site: string, user: number, version: number): Promise<TeamMember> =>
    request(`/api/ext/oem/sites/${encodeURIComponent(site)}/team/${user}/revoke`, "POST", {
      version,
    }),
  catalog: (): Promise<ModelCatalog> => request("/api/ext/oem/catalog"),
  self: (): Promise<SelfUser> => request<SelfUser>("/api/user/self"),
  status: (): Promise<Record<string, unknown>> => request<Record<string, unknown>>("/api/status"),
};

// Deliberately fixed: no query/hash/user-supplied destination is propagated.
export function officialLoginUrl(): string {
  const target = sanitizeAuthRedirect("/cmh/whoami", window.location.origin);
  if (target !== "/cmh/whoami") return "/sign-in";
  return `/sign-in?redirect=${encodeURIComponent(target)}`;
}

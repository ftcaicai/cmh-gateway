import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRootRoute, Link, Outlet } from "@tanstack/react-router";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";

import { clearAuthentication } from "@/lib/auth-session";
import { subscribeAuthSessionEvents } from "@/lib/auth-session-sync";
import { useAuthStore } from "@/stores/auth-store";
import { officialLoginUrl } from "~/lib/api";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
});

export const Route = createRootRoute({
  component: ConsoleLayout,
  notFoundComponent: NotFound,
});

function ConsoleLayout() {
  const { t } = useTranslation();
  useEffect(
    () =>
      subscribeAuthSessionEvents((event) => {
        const sid = useAuthStore.getState().auth.session?.sid;
        if (event.kind === "authenticated") {
          if (event.sid === sid) return;
          clearAuthentication(false, "idle");
          queryClient.clear();
          window.location.reload();
          return;
        }
        if (event.sid !== sid) return;
        clearAuthentication(false);
        queryClient.clear();
        window.location.replace(officialLoginUrl());
      }),
    [],
  );

  return (
    <QueryClientProvider client={queryClient}>
      <div style={{ fontFamily: "system-ui, sans-serif", margin: "2rem auto", maxWidth: 960 }}>
        <header
          style={{
            display: "flex",
            gap: 16,
            alignItems: "baseline",
            borderBottom: "1px solid #ddd",
            paddingBottom: 8,
          }}
        >
          <strong>{t("Operations console")}</strong>
          <nav style={{ display: "flex", gap: 12 }}>
            <Link to="/">{t("Overview")}</Link>
            <Link to="/whoami">{t("Current account")}</Link>
          </nav>
        </header>
        <main style={{ paddingTop: 16 }}>
          <Outlet />
        </main>
      </div>
    </QueryClientProvider>
  );
}

function NotFound() {
  const { t } = useTranslation();
  return <p>{t("Page not found.")}</p>;
}

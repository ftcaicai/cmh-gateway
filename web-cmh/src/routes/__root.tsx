import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRootRoute, Link, Outlet } from "@tanstack/react-router";
import {
  ArrowLeft,
  Boxes,
  CircleUser,
  LayoutDashboard,
  Store,
  Users,
  UsersRound,
} from "lucide-react";
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

const navigation = [
  { to: "/", label: "Overview", icon: LayoutDashboard },
  { to: "/brands", label: "Brand profiles", icon: Store },
  { to: "/customers", label: "Customers", icon: Users },
  { to: "/team", label: "Read-only team", icon: UsersRound },
  { to: "/catalog", label: "Model catalog", icon: Boxes },
  { to: "/whoami", label: "Current account", icon: CircleUser },
] as const;

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

  const links = navigation.map(({ to, label, icon: Icon }) => (
    <Link
      key={to}
      to={to}
      activeOptions={{ exact: to === "/" }}
      className="text-muted-foreground hover:bg-accent hover:text-foreground flex shrink-0 items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors"
      activeProps={{ className: "bg-accent text-foreground" }}
    >
      <Icon className="size-4" aria-hidden="true" />
      {t(label)}
    </Link>
  ));

  return (
    <QueryClientProvider client={queryClient}>
      <div className="bg-muted/40 text-foreground flex min-h-svh flex-col md:flex-row">
        <aside className="bg-background hidden w-60 shrink-0 flex-col border-r md:flex">
          <div className="flex h-14 items-center gap-2 border-b px-5 font-semibold">
            <Store className="text-primary size-5" aria-hidden="true" />
            {t("Operations console")}
          </div>
          <nav className="flex flex-1 flex-col gap-1 p-3">{links}</nav>
          <a
            href="/"
            className="text-muted-foreground hover:text-foreground flex items-center gap-2 border-t px-5 py-3 text-sm"
          >
            <ArrowLeft className="size-4" aria-hidden="true" />
            {t("Back to main site")}
          </a>
        </aside>
        <header className="bg-background border-b md:hidden">
          <div className="flex h-12 items-center justify-between px-4 font-semibold">
            {t("Operations console")}
            <a href="/" className="text-muted-foreground text-sm font-normal">
              {t("Back to main site")}
            </a>
          </div>
          <nav className="flex gap-1 overflow-x-auto px-2 pb-2">{links}</nav>
        </header>
        <main className="cmh-content mx-auto w-full max-w-5xl min-w-0 flex-1 px-4 py-6 md:px-8 md:py-8">
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

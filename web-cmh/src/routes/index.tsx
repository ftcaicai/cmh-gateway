import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Boxes, Store, Users } from "lucide-react";
import { useTranslation } from "react-i18next";

import { api } from "~/lib/api";

export const Route = createFileRoute("/")({ component: Overview });

const shortcuts = [
  {
    to: "/brands",
    title: "Brand profiles",
    hint: "Create a brand and copy its site ID.",
    icon: Store,
  },
  {
    to: "/customers",
    title: "Customers",
    hint: "Invite accounts and manage associations.",
    icon: Users,
  },
  {
    to: "/catalog",
    title: "Model catalog",
    hint: "See the models your group can use.",
    icon: Boxes,
  },
] as const;

function Overview() {
  const { t } = useTranslation();
  const status = useQuery({ queryKey: ["status"], queryFn: api.status });
  let statusText = t("Available");
  if (status.isPending) statusText = t("Loading...");
  else if (status.isError) statusText = t("Unavailable");
  return (
    <section>
      <h2>{t("Overview")}</h2>
      <p>{t("This independent console reuses the official sign-in session.")}</p>
      <p data-testid="status" role="status">
        {t("Gateway status")}: {statusText}
      </p>
      <div className="mt-6 grid gap-4 sm:grid-cols-3">
        {shortcuts.map(({ to, title, hint, icon: Icon }) => (
          <Link
            key={to}
            to={to}
            className="bg-card hover:border-primary/50 rounded-xl border p-5 transition-colors"
          >
            <Icon className="text-primary size-5" aria-hidden="true" />
            <div className="mt-3 font-medium">{t(title)}</div>
            <p className="cmh-muted">{t(hint)}</p>
          </Link>
        ))}
      </div>
    </section>
  );
}

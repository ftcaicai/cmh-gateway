import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { api } from "~/lib/api";

export const Route = createFileRoute("/")({ component: Overview });

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
      <p data-testid="status">
        {t("Gateway status")}: {statusText}
      </p>
    </section>
  );
}

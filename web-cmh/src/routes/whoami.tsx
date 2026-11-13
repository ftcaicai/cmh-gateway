import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/loading-state";
import { api, ApiError, officialLoginUrl } from "~/lib/api";

export const Route = createFileRoute("/whoami")({ component: Whoami });

function Whoami() {
  const { t } = useTranslation();
  const self = useQuery({ queryKey: ["self"], queryFn: api.self, gcTime: 0 });
  const signedOut = self.error instanceof ApiError && self.error.status === 401;

  useEffect(() => {
    if (signedOut) window.location.replace(officialLoginUrl());
  }, [signedOut]);

  if (self.isPending) return <LoadingState message={t("Loading account...")} />;
  if (signedOut) return <p role="status">{t("Session expired. Redirecting to sign in...")}</p>;
  // A refresh/network/protocol failure is not proof the user is signed out.
  // Do not redirect or render previously cached identity on any error.
  if (self.isError)
    return <p role="alert">{t("Unable to verify your session. Reload to try again.")}</p>;
  return (
    <section>
      <h2>{t("Current account")}</h2>
      <dl data-testid="auth-state">
        <dt>{t("Username")}</dt>
        <dd>{self.data.username}</dd>
        <dt>{t("Role")}</dt>
        <dd>{self.data.role}</dd>
        <dt>{t("Group")}</dt>
        <dd>{self.data.group ?? "-"}</dd>
      </dl>
    </section>
  );
}

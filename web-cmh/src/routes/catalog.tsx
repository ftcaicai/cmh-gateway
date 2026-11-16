import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { api } from "~/lib/api";
export const Route = createFileRoute("/catalog")({ component: Catalog });
function Catalog() {
  const { t } = useTranslation();
  const catalog = useQuery({
    queryKey: ["oem-catalog"],
    queryFn: api.catalog,
    gcTime: 0,
    retry: false,
  });
  return (
    <section>
      <h2>{t("Model catalog")}</h2>
      <p>{t("Publication visibility is not permission to execute a model.")}</p>
      <p>{t("Policies are immutable at startup and cannot be edited here.")}</p>
      {catalog.isPending && <p>{t("Loading...")}</p>}
      {catalog.isError && (
        <div role="alert">
          <p>{t("Unable to load model catalog.")}</p>
          <Button onClick={() => void catalog.refetch()}>{t("Retry")}</Button>
        </div>
      )}
      {catalog.data && (
        <>
          <p>
            {t("Model policy")}:{" "}
            {catalog.data.model_policy_enabled ? t("Enabled") : t("Upstream default")}
          </p>
          <p>
            {t("Destination policy")}:{" "}
            {catalog.data.destination_policy_enabled ? t("Enabled") : t("Upstream default")}
          </p>
          {catalog.data.models.length === 0 ? (
            <p>{t("No published models.")}</p>
          ) : (
            <ul>
              {catalog.data.models.map((model) => (
                <li key={model}>{model}</li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  );
}

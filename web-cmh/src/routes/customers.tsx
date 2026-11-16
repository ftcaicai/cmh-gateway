import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldError } from "@/components/ui/field";
import { CopyButton } from "@/components/copy-button";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { api, type Customer } from "~/lib/api";
import {
  invitationSchema,
  acceptanceSchema,
  type InvitationForm,
  type AcceptanceForm,
} from "~/features/customers/lib/schema";

export const Route = createFileRoute("/customers")({ component: Customers });

function Customers() {
  const { t } = useTranslation();
  const sites = useQuery({ queryKey: ["oem-sites"], queryFn: api.brands, retry: false, gcTime: 0 });
  const [site, setSite] = useState("");
  return (
    <section>
      <h2>{t("Customers")}</h2>
      <p>{t("Customer associations are management records, not relay access controls.")}</p>
      <SelfMembership />
      <h3>{t("Merchant management")}</h3>
      {sites.isPending && <p>{t("Loading brand profiles...")}</p>}
      {sites.isError && <p role="alert">{t("Unable to load brand profiles.")}</p>}
      {sites.data?.length === 0 && <p>{t("No brand profiles yet. Create your first profile.")}</p>}
      <div className="flex flex-wrap gap-2">
        {sites.data?.map((item) => (
          <Button
            key={item.id}
            variant={site === item.id ? "default" : "outline"}
            onClick={() => setSite(item.id)}
          >
            {item.name}
          </Button>
        ))}
      </div>
      {site && <MerchantCustomers key={site} site={site} />}
    </section>
  );
}

function SelfMembership() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const membership = useQuery({
    queryKey: ["oem-membership"],
    queryFn: api.membership,
    retry: false,
    gcTime: 0,
  });
  const form = useForm<AcceptanceForm>({
    resolver: zodResolver(acceptanceSchema),
    defaultValues: { token: "" },
  });
  const [result, setResult] = useState<"ok" | "error" | null>(null);
  const accept = useMutation({
    mutationFn: (token: string) => api.acceptInvitation(token),
    gcTime: 0,
    onSuccess: () => {
      setResult("ok");
      void client.invalidateQueries({ queryKey: ["oem-membership"] });
    },
    onError: () => setResult("error"),
    onSettled: () => {
      form.reset();
      accept.reset();
    },
  });
  return (
    <section>
      <h3>{t("My membership")}</h3>
      {membership.isPending && <p>{t("Loading...")}</p>}
      {membership.isError && <p role="alert">{t("Unable to load customer records.")}</p>}
      {membership.data && (
        <p>
          {membership.data.membership?.active
            ? t("Active association")
            : t("No active association")}
        </p>
      )}
      <form
        onSubmit={form.handleSubmit((values) => {
          setResult(null);
          form.reset();
          accept.mutate(values.token);
        })}
      >
        <Field data-invalid={!!form.formState.errors.token}>
          <FieldLabel htmlFor="accept-token">{t("Invitation token")}</FieldLabel>
          <Input
            id="accept-token"
            type="password"
            autoComplete="off"
            {...form.register("token")}
            disabled={accept.isPending}
            aria-invalid={!!form.formState.errors.token}
          />
          {form.formState.errors.token && (
            <FieldError>{t("Enter a valid invitation token.")}</FieldError>
          )}
        </Field>
        <Button type="submit" disabled={accept.isPending}>
          {t("Accept invitation")}
        </Button>
      </form>
      {result === "ok" && <p role="status">{t("Invitation accepted.")}</p>}
      {result === "error" && (
        <p role="alert">
          {t("Invitation could not be accepted. Check the account, token and expiry.")}
        </p>
      )}
    </section>
  );
}

function MerchantCustomers(props: { site: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const customers = useQuery({
    queryKey: ["oem-customers", props.site],
    queryFn: () => api.customers(props.site),
    retry: false,
    gcTime: 0,
  });
  const invitations = useQuery({
    queryKey: ["oem-invitations", props.site],
    queryFn: () => api.invitations(props.site),
    retry: false,
    gcTime: 0,
  });
  const audits = useQuery({
    queryKey: ["oem-customer-audits", props.site],
    queryFn: () => api.customerAudits(props.site),
    retry: false,
    gcTime: 0,
  });
  const form = useForm<InvitationForm>({
    resolver: zodResolver(invitationSchema),
    defaultValues: { target_user_id: "" },
  });
  const [secret, setSecret] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [revoking, setRevoking] = useState<Customer | null>(null);
  const invite = useMutation({
    mutationFn: (user: number) => api.inviteCustomer(props.site, user),
    gcTime: 0,
    onSuccess: (data) => {
      setSecret(data.token);
      form.reset();
      void client.invalidateQueries({ queryKey: ["oem-customer-audits", props.site] });
      void client.invalidateQueries({ queryKey: ["oem-invitations", props.site] });
    },
    onError: () => setFailed(true),
    onSettled: (): void => invite.reset(),
  });
  const revoke = useMutation({
    mutationFn: (user: number) => api.revokeCustomer(props.site, user),
    onSuccess: () => {
      setRevoking(null);
      void client.invalidateQueries({ queryKey: ["oem-customer-audits", props.site] });
      void client.invalidateQueries({ queryKey: ["oem-customers", props.site] });
    },
  });
  return (
    <section className="space-y-4">
      <h3>{t("Customer associations")}</h3>
      {customers.isPending && <p>{t("Loading...")}</p>}
      {customers.isError && <p role="alert">{t("Unable to load customer records.")}</p>}
      {customers.data?.length === 0 && <p>{t("No customer associations.")}</p>}
      <ul>
        {customers.data?.map((customer) => (
          <li key={customer.user_id}>
            {t("User ID")}: {customer.user_id} ·{" "}
            {customer.active ? t("Active association") : t("Revoked association")}{" "}
            {customer.active && (
              <Button
                variant="outline"
                onClick={() => setRevoking(customer)}
                disabled={revoke.isPending}
              >
                {t("Revoke association")}
              </Button>
            )}
          </li>
        ))}
      </ul>
      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(open) => {
          if (!open && !revoke.isPending) setRevoking(null);
        }}
        title={t("Revoke association")}
        desc={t("This removes only the management association. It does not disable relay access.")}
        confirmText={t("Revoke association")}
        isLoading={revoke.isPending}
        handleConfirm={() => {
          if (revoking) revoke.mutate(revoking.user_id);
        }}
      />
      {revoke.isError && <p role="alert">{t("Unable to revoke association.")}</p>}
      <form
        onSubmit={form.handleSubmit((values) => {
          setSecret(null);
          setFailed(false);
          invite.mutate(Number(values.target_user_id));
        })}
      >
        <Field data-invalid={!!form.formState.errors.target_user_id}>
          <FieldLabel htmlFor="target-user">{t("Target user ID")}</FieldLabel>
          <Input
            id="target-user"
            inputMode="numeric"
            {...form.register("target_user_id")}
            disabled={invite.isPending}
            aria-invalid={!!form.formState.errors.target_user_id}
          />
          {form.formState.errors.target_user_id && (
            <FieldError>{t("Enter a positive user ID.")}</FieldError>
          )}
        </Field>
        <Button type="submit" disabled={invite.isPending}>
          {t("Create invitation")}
        </Button>
      </form>
      {failed && (
        <p role="alert">{t("Invitation creation failed. Check records before trying again.")}</p>
      )}
      {secret && (
        <section aria-label={t("One-time invitation")}>
          <p>{t("Copy now. The token is shown once and expires in 24 hours.")}</p>
          <CopyButton value={secret} aria-label={t("Copy invitation token")} />
          <Button onClick={() => setSecret(null)}>{t("Close")}</Button>
        </section>
      )}
      <h3>{t("Invitations")}</h3>
      {invitations.isPending && <p>{t("Loading...")}</p>}
      {invitations.isError && <p role="alert">{t("Unable to load customer records.")}</p>}
      {invitations.data?.length === 0 && <p>{t("No invitations.")}</p>}
      <ul>
        {invitations.data?.map((item) => (
          <li key={item.id}>
            {t("User ID")}: {item.target_user_id} ·{" "}
            {item.accepted_at ? t("Accepted") : t("Not accepted")}
          </li>
        ))}
      </ul>
      <h3>{t("Operation records")}</h3>
      {audits.isPending && <p>{t("Loading operation records...")}</p>}
      {audits.isError && <p role="alert">{t("Unable to load customer records.")}</p>}
      {audits.data?.length === 0 && <p>{t("No operation records.")}</p>}
      <ul>
        {audits.data?.map((record) => {
          let action = "Operation records";
          if (record.action === "customer.invited") action = "Create invitation";
          if (record.action === "customer.accepted") action = "Accept invitation";
          if (record.action === "customer.revoked") action = "Revoke association";
          return (
            <li key={record.id}>
              {t(action)} · {t("User ID")}: {record.user_id}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

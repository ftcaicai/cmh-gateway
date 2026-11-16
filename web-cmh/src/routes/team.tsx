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
import { api, ApiError, ownsBrand, type TeamMember } from "~/lib/api";
import {
  acceptanceSchema,
  invitationSchema,
  type AcceptanceForm,
  type InvitationForm,
} from "~/features/customers/lib/schema";

export const Route = createFileRoute("/team")({ component: Team });

function Team() {
  const { t } = useTranslation();
  const sites = useQuery({ queryKey: ["oem-sites"], queryFn: api.brands, retry: false, gcTime: 0 });
  const [site, setSite] = useState("");
  const owned = sites.data?.filter(ownsBrand) ?? [];
  return (
    <section className="space-y-4">
      <h2>{t("Read-only team")}</h2>
      <p>
        {t(
          "Invite existing platform Admin accounts only. Collaboration grants no write access or private notes.",
        )}
      </p>
      <TeamAcceptance />
      <h3>{t("Manage owned brand team")}</h3>
      {sites.isPending && <p>{t("Loading brand profiles...")}</p>}
      {sites.isError && <p role="alert">{t("Unable to load brand profiles.")}</p>}
      {sites.isSuccess && owned.length === 0 && <p>{t("No owned brands available.")}</p>}
      <div className="flex flex-wrap gap-2">
        {owned.map((item) => (
          <Button
            key={item.id}
            variant={site === item.id ? "default" : "outline"}
            onClick={() => setSite(item.id)}
          >
            {item.name}
          </Button>
        ))}
      </div>
      {owned.some((item) => item.id === site) && <OwnerTeam key={site} site={site} />}
    </section>
  );
}

function TeamAcceptance() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const form = useForm<AcceptanceForm>({
    resolver: zodResolver(acceptanceSchema),
    defaultValues: { token: "" },
  });
  const [result, setResult] = useState<"ok" | "error" | null>(null);
  const accept = useMutation({
    mutationFn: api.acceptTeamInvitation,
    retry: false,
    gcTime: 0,
    onSuccess: () => {
      setResult("ok");
      void client.invalidateQueries({ queryKey: ["oem-sites"] });
    },
    onError: () => setResult("error"),
    onSettled: () => {
      form.reset();
      accept.reset();
    },
  });
  return (
    <section>
      <h3>{t("Accept team invitation")}</h3>
      <form
        onSubmit={form.handleSubmit((values) => {
          if (accept.isPending) return;
          setResult(null);
          form.reset();
          accept.mutate(values.token);
        })}
      >
        <Field data-invalid={!!form.formState.errors.token}>
          <FieldLabel htmlFor="team-accept-token">{t("Invitation token")}</FieldLabel>
          <Input
            id="team-accept-token"
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
          {t("Accept team invitation")}
        </Button>
      </form>
      {result === "ok" && (
        <p role="status">
          {t("Team invitation accepted. Shared brands are available in profiles and customers.")}
        </p>
      )}
      {result === "error" && (
        <p role="alert">
          {t("Invitation could not be accepted. Check the account, token and expiry.")}
        </p>
      )}
    </section>
  );
}

function OwnerTeam(props: { site: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const members = useQuery({
    queryKey: ["oem-team", props.site],
    queryFn: () => api.team(props.site),
    retry: false,
    gcTime: 0,
  });
  const invitations = useQuery({
    queryKey: ["oem-team-invitations", props.site],
    queryFn: () => api.teamInvitations(props.site),
    retry: false,
    gcTime: 0,
  });
  const audits = useQuery({
    queryKey: ["oem-team-audits", props.site],
    queryFn: () => api.teamAudits(props.site),
    retry: false,
    gcTime: 0,
  });
  const form = useForm<InvitationForm>({
    resolver: zodResolver(invitationSchema),
    defaultValues: { target_user_id: "" },
  });
  const [secret, setSecret] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [revoking, setRevoking] = useState<TeamMember | null>(null);
  const refresh = () => {
    for (const key of ["oem-team", "oem-team-invitations", "oem-team-audits"])
      void client.invalidateQueries({ queryKey: [key, props.site] });
  };
  const invite = useMutation({
    mutationFn: (user: number) => api.inviteTeamMember(props.site, user),
    retry: false,
    gcTime: 0,
    onSuccess: (data) => {
      setSecret(data.token);
      form.reset();
      refresh();
    },
    onError: () => setFailed(true),
    onSettled: (): void => invite.reset(),
  });
  const revoke = useMutation({
    mutationFn: (member: TeamMember) =>
      api.revokeTeamMember(props.site, member.user_id, member.version),
    retry: false,
    gcTime: 0,
    onSuccess: () => {
      setRevoking(null);
      refresh();
    },
  });
  return (
    <section className="space-y-4">
      <h3>{t("Team members")}</h3>
      {members.isPending && <p>{t("Loading...")}</p>}
      {members.isError && <p role="alert">{t("Unable to load team records.")}</p>}
      {members.data?.length === 0 && <p>{t("No team members.")}</p>}
      <ul>
        {members.data?.map((member) => (
          <li key={member.user_id}>
            {t("User ID")}: {member.user_id} ·{" "}
            {member.active ? t("Shared read-only access") : t("Revoked association")}{" "}
            {member.active && (
              <Button
                variant="outline"
                disabled={revoke.isPending}
                onClick={() => {
                  revoke.reset();
                  setRevoking(member);
                }}
              >
                {t("Revoke team access")}
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
        title={t("Revoke team access")}
        desc={t(
          "The next request will lose access to this shared brand. The official account is unchanged.",
        )}
        confirmText={t("Revoke team access")}
        isLoading={revoke.isPending}
        disabled={revoke.isError}
        handleConfirm={() => {
          if (revoking && !revoke.isPending && !revoke.isError) revoke.mutate(revoking);
        }}
      />
      {revoke.isError && (
        <p role="alert">
          {revoke.error instanceof ApiError && revoke.error.status === 409
            ? t("Team membership changed. Reload records before trying again.")
            : t("Team action failed. Check records before trying again.")}
        </p>
      )}
      <Button
        variant="outline"
        onClick={() => {
          revoke.reset();
          setRevoking(null);
          refresh();
        }}
      >
        {t("Reload")}
      </Button>
      <form
        onSubmit={form.handleSubmit((values) => {
          if (invite.isPending) return;
          setSecret(null);
          setFailed(false);
          invite.mutate(Number(values.target_user_id));
        })}
      >
        <Field data-invalid={!!form.formState.errors.target_user_id}>
          <FieldLabel htmlFor="team-target-user">{t("Target Admin user ID")}</FieldLabel>
          <Input
            id="team-target-user"
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
          {t("Create team invitation")}
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
      {invitations.isError && <p role="alert">{t("Unable to load team records.")}</p>}
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
      {audits.isError && <p role="alert">{t("Unable to load team records.")}</p>}
      {audits.data?.length === 0 && <p>{t("No operation records.")}</p>}
      <ul>
        {audits.data?.map((record) => {
          let action = "Operation records";
          if (record.action === "team.invited") action = "Create team invitation";
          if (record.action === "team.accepted") action = "Accept team invitation";
          if (record.action === "team.revoked") action = "Revoke team access";
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

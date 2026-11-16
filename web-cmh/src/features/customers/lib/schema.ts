import { z } from "zod";

export const invitationSchema = z.object({
  target_user_id: z
    .string()
    .regex(/^[1-9]\d*$/)
    .refine((value) => Number.isSafeInteger(Number(value))),
});
export const acceptanceSchema = z.object({ token: z.string().regex(/^[a-f0-9]{64}$/) });
export type InvitationForm = z.infer<typeof invitationSchema>;
export type AcceptanceForm = z.infer<typeof acceptanceSchema>;

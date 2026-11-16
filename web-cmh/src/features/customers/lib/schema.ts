import { z } from "zod";

export const invitationSchema = z.object({
  target_user_id: z
    .string()
    .regex(/^[1-9]\d*$/)
    .refine((value) => Number.isSafeInteger(Number(value))),
});
export const acceptanceSchema = z.object({ token: z.string().regex(/^[a-f0-9]{64}$/) });
export const customerNoteSchema = z.object({
  note: z.string().refine((value) => {
    if (new TextEncoder().encode(value).byteLength > 2000) return false;
    for (const character of value) {
      const code = character.codePointAt(0)!;
      if ((code < 32 && code !== 9 && code !== 10) || (code >= 127 && code <= 159)) return false;
    }
    return true;
  }),
});
export type CustomerNoteForm = z.infer<typeof customerNoteSchema>;
export type InvitationForm = z.infer<typeof invitationSchema>;
export type AcceptanceForm = z.infer<typeof acceptanceSchema>;

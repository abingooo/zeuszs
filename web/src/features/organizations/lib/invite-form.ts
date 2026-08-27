/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { z } from 'zod'

export function getCreateInviteSchema(t: (key: string) => string) {
  return z
    .object({
      code: z
        .string()
        .trim()
        .refine(
          (value) => value === '' || (value.length >= 8 && value.length <= 64),
          t('Invite code must be 8-64 characters')
        ),
      max_uses: z
        .number()
        .int(t('Maximum uses must be a whole number'))
        .min(0, t('Maximum uses cannot be negative'))
        .max(1_000_000_000),
      expires_at: z.string(),
    })
    .refine(
      (values) =>
        values.expires_at === '' ||
        new Date(values.expires_at).getTime() > Date.now(),
      {
        path: ['expires_at'],
        message: t('Expiration must be in the future'),
      }
    )
}

export type CreateInviteFormValues = z.infer<
  ReturnType<typeof getCreateInviteSchema>
>

export const CREATE_INVITE_DEFAULTS: CreateInviteFormValues = {
  code: '',
  max_uses: 0,
  expires_at: '',
}

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

export function getCreateOrganizationSchema(t: (key: string) => string) {
  return z.object({
    name: z.string().trim().min(1, t('Organization name is required')).max(128),
    owner_username: z.string().trim().min(1, t('Owner username is required')),
    owner_password: z
      .string()
      .min(8, t('Password must be between 8 and 20 characters'))
      .max(20, t('Password must be between 8 and 20 characters')),
    owner_display_name: z.string().trim().max(128),
    owner_email: z
      .string()
      .trim()
      .refine(
        (value) => value === '' || z.email().safeParse(value).success,
        t('Please enter a valid email address')
      ),
    allow_member_topup: z.boolean(),
  })
}

export type CreateOrganizationFormValues = z.infer<
  ReturnType<typeof getCreateOrganizationSchema>
>

export const CREATE_ORGANIZATION_DEFAULTS: CreateOrganizationFormValues = {
  name: '',
  owner_username: '',
  owner_password: '',
  owner_display_name: '',
  owner_email: '',
  allow_member_topup: true,
}

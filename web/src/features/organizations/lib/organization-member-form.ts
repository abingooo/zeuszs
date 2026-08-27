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

export const ORGANIZATION_MEMBER_ROLE_OPTIONS = ['admin', 'member'] as const

export function getProvisionOrganizationMemberSchema(
  t: (key: string) => string
) {
  return z.object({
    username: z
      .string()
      .trim()
      .min(1, t('Username is required'))
      .max(20, t('Username must be at most 20 characters')),
    password: z
      .string()
      .min(8, t('Password must be between 8 and 20 characters'))
      .max(20, t('Password must be between 8 and 20 characters')),
    display_name: z
      .string()
      .trim()
      .max(20, t('Display name must be at most 20 characters')),
    email: z
      .string()
      .trim()
      .max(50, t('Email must be at most 50 characters'))
      .refine(
        (value) => value === '' || z.email().safeParse(value).success,
        t('Please enter a valid email address')
      ),
    organization_role: z.enum(ORGANIZATION_MEMBER_ROLE_OPTIONS),
  })
}

export type ProvisionOrganizationMemberFormValues = z.infer<
  ReturnType<typeof getProvisionOrganizationMemberSchema>
>

export const PROVISION_ORGANIZATION_MEMBER_DEFAULTS: ProvisionOrganizationMemberFormValues =
  {
    username: '',
    password: '',
    display_name: '',
    email: '',
    organization_role: 'admin',
  }

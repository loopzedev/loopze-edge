// Mirror of the public-facing User payload from the Go backend.
// Note: PasswordHash is intentionally absent — the server never sends it.

export type Role = 'admin' | 'editor' | 'viewer'
export type AuthProvider = 'local'

export interface User {
  id: string
  username: string
  role: Role
  authProvider: AuthProvider
  disabled: boolean
  createdAt: string
  updatedAt: string
}

// Permissions checked via authStore.can(action). Kept as a tagged union
// so adding a new permission forces every call site to update too.
export type AuthAction =
  | 'deploy'
  | 'inject'
  | 'manageUsers'
  | 'mutateContext'

const ROLE_RANK: Record<Role, number> = {
  viewer: 1,
  editor: 2,
  admin: 3,
}

const ACTION_MIN_RANK: Record<AuthAction, number> = {
  deploy: ROLE_RANK.editor,
  inject: ROLE_RANK.editor,
  mutateContext: ROLE_RANK.editor,
  manageUsers: ROLE_RANK.admin,
}

export function roleHasAction(role: Role, action: AuthAction): boolean {
  return ROLE_RANK[role] >= ACTION_MIN_RANK[action]
}

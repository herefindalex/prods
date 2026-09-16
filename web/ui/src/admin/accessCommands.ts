import { api, postJSON, putJSON } from "./api";
import type { Capability, Role, User } from "./types";

export type GrantResponse = { user: User; set_password_url: string; expires_at: string };
export type InvitationMailStatus = "pending" | "accepted" | "failed" | "unknown";
export type InvitationMailAttempt = {
  id: string;
  user_id: string;
  recipient_email: string;
  status: InvitationMailStatus;
  error_class?: string;
  error_message?: string;
  created_at: string;
  completed_at?: string;
};

const userPath = (id: string) => `/admin/api/users/${encodeURIComponent(id)}`;

// Set-password grants are returned only to the caller and stay outside the
// Refine query cache. Security-sensitive lifecycle changes remain commands.
export const accessCommands = {
  createRole: (values: { name: string; capabilities: Capability[] }) => postJSON<Role>("/admin/api/roles", values),
  createUser: (values: { email: string; display_name: string; role_id: string }) => postJSON<GrantResponse>("/admin/api/users", values),
  changeRole: (user: User, roleID: string) => putJSON<User>(`${userPath(user.id)}/role`, { role_id: roleID }),
  issueSetPasswordGrant: (user: User) => postJSON<GrantResponse>(`${userPath(user.id)}/set-password-grant`, {}),
  reactivateUser: (user: User) => postJSON<GrantResponse>(`${userPath(user.id)}/reactivate`, {}),
  disableUser: (user: User) => postJSON<void>(`${userPath(user.id)}/disable`, {}),
  sendInvitation: (grant: GrantResponse) => postJSON<InvitationMailAttempt>(`${userPath(grant.user.id)}/send-set-password`, {
    set_password_url: grant.set_password_url,
  }),
  invitationHistory: (user: User) => api<InvitationMailAttempt[]>(`${userPath(user.id)}/invitation-mail-attempts`),
};

export const accessInvalidations = {
  roles: ["roles"],
  users: ["users"],
} as const;

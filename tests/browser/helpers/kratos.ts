// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

/**
 * Manage Kratos identities via the admin API (localhost:4434).
 */

const KRATOS_ADMIN = "http://localhost:4434";

export interface CreateIdentityOpts {
  email: string;
  password: string;
  name?: string;
  surname?: string;
}

/** Create a Kratos identity with password credentials. Returns the identity id. */
export async function createIdentity(
  opts: CreateIdentityOpts,
): Promise<string> {
  const body = {
    schema_id: "default",
    credentials: { password: { config: { password: opts.password } } },
    traits: {
      email: opts.email,
      name: opts.name ?? "Test",
      surname: opts.surname ?? "User",
    },
  };

  const res = await fetch(`${KRATOS_ADMIN}/admin/identities`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(
      `failed to create identity ${opts.email}: ${res.status} ${text}`,
    );
  }

  const data = (await res.json()) as { id: string };
  return data.id;
}

/** Delete the identity with the given id. */
export async function deleteIdentity(id: string): Promise<void> {
  const res = await fetch(`${KRATOS_ADMIN}/admin/identities/${id}`, {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 404) {
    throw new Error(`failed to delete identity ${id}: ${res.status}`);
  }
}

/** Delete all sessions for the given identity. */
export async function deleteIdentitySessions(id: string): Promise<void> {
  await fetch(`${KRATOS_ADMIN}/admin/identities/${id}/sessions`, {
    method: "DELETE",
  });
}

/** Find a Kratos identity ID by email trait. Returns null if not found. */
export async function findIdentityByEmail(
  email: string,
): Promise<string | null> {
  const res = await fetch(
    `${KRATOS_ADMIN}/admin/identities?per_page=200`,
  );
  if (!res.ok) return null;

  const identities = (await res.json()) as Array<{
    id: string;
    traits?: { email?: string };
  }>;
  for (const identity of identities) {
    if (identity.traits?.email === email) {
      return identity.id;
    }
  }
  return null;
}

export interface CreateIdentityWithOIDCOpts {
  email: string;
  provider: string;
  subject: string;
}

/**
 * Create a Kratos identity with OIDC credentials pre-linked.
 *
 * This lets the identifier-first 1FA page show the OIDC provider button
 * without needing a prior OIDC registration flow.
 */
export async function createIdentityWithOIDC(
  opts: CreateIdentityWithOIDCOpts,
): Promise<string> {
  const body = {
    schema_id: "default",
    credentials: {
      // Password credentials make the email a searchable credential identifier
      // so the identifier-first flow can find the identity by email.
      password: { config: { password: "oidc-identity-unused-pw" } },
      oidc: {
        config: {
          providers: [
            {
              provider: opts.provider,
              subject: opts.subject,
            },
          ],
        },
      },
    },
    traits: {
      email: opts.email,
      name: "Dex",
      surname: "User",
    },
  };

  const res = await fetch(`${KRATOS_ADMIN}/admin/identities`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(
      `failed to create OIDC identity ${opts.email}: ${res.status} ${text}`,
    );
  }

  const data = (await res.json()) as { id: string };
  return data.id;
}

// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Manage tenants via the tenant-service API (localhost:8000).
 *
 * For protected endpoints we need a JWT from Hydra's client-credentials flow.
 * The lookup endpoint is unauthenticated.
 */

const TENANT_SERVICE = "http://localhost:8000";
const HYDRA_PUBLIC = "http://localhost:4444";

/** Obtain a client-credentials JWT for the tenant-service API. */
export async function getServiceToken(
  clientId: string,
  clientSecret: string,
): Promise<string> {
  const res = await fetch(`${HYDRA_PUBLIC}/oauth2/token`, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Authorization:
        "Basic " +
        Buffer.from(`${clientId}:${clientSecret}`).toString("base64"),
    },
    body: "grant_type=client_credentials&scope=tenant-service",
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`failed to get service token: ${res.status} ${text}`);
  }

  const data = (await res.json()) as { access_token: string };
  return data.access_token;
}

export interface Tenant {
  id: string;
  name: string;
}

/** Create a tenant. Returns the tenant object. */
export async function createTenant(
  token: string,
  name: string,
): Promise<Tenant> {
  const res = await fetch(`${TENANT_SERVICE}/api/v0/tenants`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ name }),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`failed to create tenant ${name}: ${res.status} ${text}`);
  }

  const data = (await res.json()) as { tenant: Tenant };
  return data.tenant;
}

/** Delete a tenant. Idempotent. */
export async function deleteTenant(
  token: string,
  tenantId: string,
): Promise<void> {
  const res = await fetch(`${TENANT_SERVICE}/api/v0/tenants/${tenantId}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok && res.status !== 404) {
    throw new Error(`failed to delete tenant ${tenantId}: ${res.status}`);
  }
}

/** Provision a user into a tenant (by email). */
export async function provisionUser(
  token: string,
  tenantId: string,
  email: string,
  role: string = "member",
): Promise<void> {
  const res = await fetch(
    `${TENANT_SERVICE}/api/v0/tenants/${tenantId}/users`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ email, role }),
    },
  );

  if (!res.ok) {
    const text = await res.text();
    throw new Error(
      `failed to provision ${email} to ${tenantId}: ${res.status} ${text}`,
    );
  }
}

/** Lookup tenants by email or identity_id (unauthenticated). */
export async function lookupTenants(
  email: string,
): Promise<Tenant[]> {
  const res = await fetch(
    `${TENANT_SERVICE}/api/v0/tenants/lookup?email=${encodeURIComponent(email)}`,
  );

  if (!res.ok) {
    const text = await res.text();
    throw new Error(
      `failed to lookup tenants for ${email}: ${res.status} ${text}`,
    );
  }

  const data = (await res.json()) as { tenants: Tenant[] };
  return data.tenants;
}

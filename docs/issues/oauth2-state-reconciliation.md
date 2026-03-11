# Issue: OAuth2 Client State Consistency & Dual-Write Reconciliation

## Context

The Tenant Service introduces support for managing machine-to-machine (M2M) OAuth2 clients via the `CreateTenantClient` and `DeleteTenantClient` endpoints.

This design employs a **Dual-Write Pattern**. We must insert/delete data across two independent persistent stores simultaneously:
1. **Hydra Admin API**: Creating or deleting the actual `client_credentials` record.
2. **PostgreSQL (`memberships` table)**: Recording the relationship between the tenant and the client (acting as a member).

Currently, this is orchestrated synchronously inside a database transaction:
1. Begin DB Transaction.
2. Insert membership tuple into PostgreSQL.
3. Call Hydra Admin API to create the OAuth2 client.
4. If Hydra succeeds, Commit DB Transaction.
5. If Hydra fails, Rollback DB Transaction.

## The Problem: Inconsistency and "Zombie" Clients

Synchronous dual-writes across distributed systems without two-phase commit (2PC) inevitably lead to edge case inconsistencies.

### 1. Database Commit Failure (The "Zombie" Client)
If the Hydra API call (Step 3) succeeds, the OAuth2 client is provisioned in Hydra. If the subsequent `COMMIT` to PostgreSQL (Step 4) fails (e.g., due to a network blip, connection pool exhaustion, or database constraint violation), the database transaction rolls back.

**Result**: We now have a "Zombie" client in Hydra. It has a valid `client_id` and `client_secret` but exists nowhere in our database. It is effectively orphaned, and our system has no record of it to display or manage it.

### 2. Deletion Rollbacks (The "Ghost" Client)
The same flaw applies in reverse during `DeleteTenantClient`. If we successfully call Hydra to delete the client, but the DB rollback triggers on the transaction, our `memberships` table will still claim the client exists.

**Result**: We have a "Ghost" client. Our API will list it, but any attempt to delete it again or use it will fail because Hydra will return a `404 Not Found`.

### 3. Orphaned Clients on Tenant Deletion
Currently, when a tenant is deleted, the `memberships` table's foreign keys automatically drop the membership rows for all clients. However, the system does not reach out to Hydra to delete the actual client records. These clients remain alive in Hydra but with no tenant associations in our DB.

## Proposal: Reconciliation Mechanism

To guarantee long-term consistency, we must introduce a background reconciliation task (Garbage Collection).

### Method: Tag-and-Sweep Cron Job

Since we trust the PostgreSQL database as the single source of truth for authorization and relationships, we can use an asynchronous reconciliation loop to clean up Hydra.

1. **Tagging**: During `CreateTenantClient`, we write a metadata tag to the Hydra client: `{"managed_by": "tenant-service", "tenant_id": "<ID>"}`.
2. **Periodic Sweep**:
   A background worker (running via cron or as an internal goroutine) periodically queries the Hydra API for all clients managed by the service: `GET /clients`.
3. **Diffing**: For each client fetched from Hydra, query the primary database:
   `SELECT 1 FROM memberships WHERE identity_id = ? AND identity_type = 'client';`
4. **Pruning**: If the client exists in Hydra but is missing from the database (either due to a failed DB commit during creation, or because its parent tenant was deleted), the worker issues a `DELETE` request to Hydra to remove the orphaned client.

**Why this approach?**
An "Outbox Pattern" (relaying creation events from a DB table to a worker) is often preferred for eventual consistency, but it does not work well here because our API needs to synchronously return the generated `client_secret` to the user in the HTTP response. We must call Hydra synchronously. Therefore, a post-hoc Cron Reconciler is the most pragmatic approach to handle failures and tenant cascades.

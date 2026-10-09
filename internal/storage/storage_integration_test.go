// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/testhelpers"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/migrations"
)

// pgErrCodeCheckViolation is the SQLSTATE of a failed CHECK constraint.
const pgErrCodeCheckViolation = "23514"

var shared testhelpers.SharedContainers

func TestMain(m *testing.M) {
	code := m.Run()
	shared.Close()
	os.Exit(code)
}

// newIntegrationStorage returns a Storage on a fresh, migrated database of
// the package's Postgres container, and the client it runs on.
func newIntegrationStorage(t *testing.T) (*Storage, *db.DBClient) {
	t.Helper()

	cfg := db.Config{DSN: shared.Postgres.IsolatedDB(t), MaxConns: 5, MinConns: 1}

	logger := logging.NewNoopLogger()
	monitor := monitoring.NewNoopMonitor("tenant-service-test", logger)
	tracer := tracing.NewNoopTracer()

	client, err := db.NewDBClient(cfg, tracer, monitor, logger)
	if err != nil {
		t.Fatalf("failed to create DB client: %v", err)
	}
	t.Cleanup(client.Close)

	return NewStorage(client, tracer, monitor, logger), client
}

func newID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func newTenant(t *testing.T, s *Storage, name string, enabled bool) *types.Tenant {
	t.Helper()
	tenant, err := s.CreateTenant(context.Background(), &types.Tenant{Name: name, Enabled: enabled})
	if err != nil {
		t.Fatalf("failed to create tenant %s: %v", name, err)
	}
	return tenant
}

// unwritten reports a policy that was never written: off, nothing bound, no
// domains.
func unwritten(p *types.TenantSSOPolicy) bool {
	return p != nil && p.Enforcement == types.EnforcementOff && !p.AutoJoin &&
		len(p.Bindings) == 0 && len(p.Domains) == 0
}

// putPolicy writes every part of p the way a request writes one: in a
// transaction that takes the policy's lock first and is rolled back when a
// write fails. It returns the policy as stored afterwards.
func putPolicy(client *db.DBClient, s *Storage, p *types.TenantSSOPolicy) (*types.TenantSSOPolicy, error) {
	ctx := context.Background()
	err := client.WithTx(ctx, func(ctx context.Context) error {
		if _, _, err := s.LockTenantSSOPolicy(ctx, p.TenantID); err != nil {
			return err
		}
		if err := s.UpdateTenantSSODomains(ctx, p.TenantID, p.Domains); err != nil {
			return err
		}
		return s.UpdateTenantSSOPolicy(ctx, p.TenantID, p.Enforcement, p.AutoJoin, p.Bindings)
	})
	if err != nil {
		return nil, err
	}
	return s.GetTenantSSOPolicy(ctx, p.TenantID)
}

func TestIntegration_MigrationsDownAndUp(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg, err := pgx.ParseConfig(shared.Postgres.IsolatedDB(t))
	if err != nil {
		t.Fatalf("failed to parse DSN: %v", err)
	}
	sqlDB := stdlib.OpenDB(*cfg)
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.EmbedMigrations)
	if err != nil {
		t.Fatalf("failed to create goose provider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
}

func TestIntegration_PersonalTenants(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, _ := newIntegrationStorage(t)
	ctx := context.Background()
	account := newID(t)

	if _, err := s.GetPersonalTenantByUserID(ctx, account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	// An organisation created before the personal tenant: its id sorts first.
	older := newTenant(t, s, "Initech", true)
	if older.IsPersonal() || older.MFARequirement != types.MFARequirementNone {
		t.Fatalf("a new organisation: %+v", older)
	}

	personal, created, err := s.CreatePersonalTenant(ctx, account, "ivy's Org")
	if err != nil || !created || personal.ID == "" || !personal.Enabled || !personal.IsPersonal() || *personal.PersonalIdentityID != account || personal.MFARequirement != types.MFARequirementNone {
		t.Fatalf("got %+v, %v, %v", personal, created, err)
	}

	// A repeat finds the first: no second tenant.
	again, created, err := s.CreatePersonalTenant(ctx, account, "ivy's Org")
	if err != nil || created || again.ID != personal.ID || !again.IsPersonal() {
		t.Fatalf("got %+v, %v, %v", again, created, err)
	}
	if got, err := s.GetPersonalTenantByUserID(ctx, account); err != nil || got.ID != personal.ID {
		t.Fatalf("got %+v, %v", got, err)
	}

	newer := newTenant(t, s, "Hooli", true)
	disabled := newTenant(t, s, "Disabled", false)
	for _, tenant := range []*types.Tenant{older, personal, newer, disabled} {
		if _, err := s.AddMember(ctx, tenant.ID, account); err != nil {
			t.Fatal(err)
		}
	}

	// Every query that returns a tenant tells a personal tenant from an
	// organisation.
	got, err := s.GetTenantByID(ctx, personal.ID)
	if err != nil || !got.IsPersonal() || got.MFARequirement != types.MFARequirementNone {
		t.Fatalf("got %+v, %v", got, err)
	}
	all, _, err := s.ListTenants(ctx)
	if err != nil || len(all) != 4 {
		t.Fatalf("got %+v, %v", all, err)
	}
	mine, err := s.ListTenantsByUserID(ctx, account)
	if err != nil || len(mine) != 4 {
		t.Fatalf("got %+v, %v", mine, err)
	}
	for _, tenant := range append(all, mine...) {
		if tenant.IsPersonal() != (tenant.ID == personal.ID) || tenant.MFARequirement != types.MFARequirementNone {
			t.Fatalf("got %+v", tenant)
		}
	}

	// The account's tenants: its personal tenant first, then the rest by id.
	wantOrder := []string{personal.ID, older.ID, newer.ID, disabled.ID}
	for i, tenant := range mine {
		if tenant.ID != wantOrder[i] {
			t.Fatalf("position %d: want %s, got %s", i, wantOrder[i], tenant.ID)
		}
	}
	// The same order among the enabled ones, as the lookup lists them.
	enabled, err := s.ListTenantsByUserID(ctx, account, types.WithEnabled(true))
	if err != nil || len(enabled) != 3 || enabled[0].ID != personal.ID || enabled[1].ID != older.ID || enabled[2].ID != newer.ID {
		t.Fatalf("got %+v, %v", enabled, err)
	}

	// Another member of the organisations has no personal tenant to list first.
	colleague := newID(t)
	for _, tenant := range []*types.Tenant{newer, older} {
		if _, err := s.AddMember(ctx, tenant.ID, colleague); err != nil {
			t.Fatal(err)
		}
	}
	theirs, err := s.ListTenantsByUserID(ctx, colleague)
	if err != nil || len(theirs) != 2 || theirs[0].ID != older.ID || theirs[1].ID != newer.ID {
		t.Fatalf("got %+v, %v", theirs, err)
	}
}

func TestIntegration_SSOPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()

	org := newTenant(t, s, "Hooli", true)
	connA, connB := newID(t), newID(t)
	if connA > connB {
		connA, connB = connB, connA
	}

	if got, err := s.GetTenantSSOPolicy(ctx, org.ID); err != nil || !unwritten(got) || got.TenantID != org.ID {
		t.Fatalf("a tenant starts with no policy written: %+v, %v", got, err)
	}

	p1, err := putPolicy(client, s, &types.TenantSSOPolicy{
		TenantID: org.ID, Enforcement: types.EnforcementRequired, AutoJoin: true,
		Domains:  []string{"hooli.example", "hooli-corp.example"},
		Bindings: []types.SSOBinding{{ConnectionID: connB}, {ConnectionID: connA, Active: true}},
	})
	if err != nil {
		t.Fatalf("got %+v, %v", p1, err)
	}

	// The read: the row with its bindings and domains, each in a fixed order.
	got, err := s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || got.TenantID != org.ID || !got.AutoJoin || !got.RequiresSSO() {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(got.Bindings) != 2 || got.Bindings[0] != (types.SSOBinding{ConnectionID: connA, Active: true}) || got.Bindings[1] != (types.SSOBinding{ConnectionID: connB}) {
		t.Fatalf("bindings by connection id, got %+v", got.Bindings)
	}
	if len(got.Domains) != 2 || got.Domains[0] != "hooli-corp.example" || got.Domains[1] != "hooli.example" {
		t.Fatalf("domains in order, got %+v", got.Domains)
	}

	// Auto-join candidates: REQUIRED, auto-join, active binding, domain.
	candidates, err := s.ListAutoJoinCandidatesByDomain(ctx, "hooli.example", "")
	if err != nil || len(candidates) != 1 || candidates[0].ID != org.ID || !candidates[0].AutoJoinCandidate {
		t.Fatalf("got %+v, %v", candidates, err)
	}

	// Each part is written alone and leaves the others as they were: the
	// domains,
	err = client.WithTx(ctx, func(ctx context.Context) error {
		tenant, locked, err := s.LockTenantSSOPolicy(ctx, org.ID)
		if err != nil || tenant.ID != org.ID || tenant.IsPersonal() || len(locked.Bindings) != 2 || len(locked.Domains) != 2 {
			return fmt.Errorf("the lock returns the tenant and its stored policy: %+v, %+v, %w", tenant, locked, err)
		}
		return s.UpdateTenantSSODomains(ctx, org.ID, []string{"hooli.test"})
	})
	got, _ = s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || len(got.Domains) != 1 || got.Domains[0] != "hooli.test" || len(got.Bindings) != 2 || !got.AutoJoin || !got.RequiresSSO() {
		t.Fatalf("only the domains change: %+v, %v", got, err)
	}
	// one binding removed,
	err = client.WithTx(ctx, func(ctx context.Context) error {
		if _, _, err := s.LockTenantSSOPolicy(ctx, org.ID); err != nil {
			return err
		}
		return s.DeleteTenantSSOBinding(ctx, org.ID, connB)
	})
	got, _ = s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || len(got.Bindings) != 1 || got.Bindings[0].ConnectionID != connA || len(got.Domains) != 1 || !got.AutoJoin {
		t.Fatalf("only the binding goes: %+v, %v", got, err)
	}

	// a binding the policy does not have: nothing changes,
	err = client.WithTx(ctx, func(ctx context.Context) error {
		if _, _, err := s.LockTenantSSOPolicy(ctx, org.ID); err != nil {
			return err
		}
		return s.DeleteTenantSSOBinding(ctx, org.ID, connB)
	})
	got, _ = s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || len(got.Bindings) != 1 || got.Bindings[0].ConnectionID != connA || len(got.Domains) != 1 || !got.AutoJoin {
		t.Fatalf("nothing changes: %+v, %v", got, err)
	}
	// and the enforcement, auto-join and bindings: A inactive, B active, no
	// auto-join.
	err = client.WithTx(ctx, func(ctx context.Context) error {
		if _, _, err := s.LockTenantSSOPolicy(ctx, org.ID); err != nil {
			return err
		}
		return s.UpdateTenantSSOPolicy(ctx, org.ID, types.EnforcementRequired, false, []types.SSOBinding{{ConnectionID: connA}, {ConnectionID: connB, Active: true}})
	})
	got, _ = s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || len(got.Bindings) != 2 || got.Bindings[0].Active || !got.Bindings[1].Active || got.AutoJoin || !got.RequiresSSO() ||
		len(got.Domains) != 1 || got.Domains[0] != "hooli.test" {
		t.Fatalf("only the enforcement, auto-join and bindings change: %+v, %v", got, err)
	}

	// Every part at once: B active, A gone, OPTIONAL, no domains.
	p2, err := putPolicy(client, s, &types.TenantSSOPolicy{
		TenantID: org.ID, Enforcement: types.EnforcementOptional,
		Bindings: []types.SSOBinding{{ConnectionID: connB, Active: true}},
	})
	if err != nil {
		t.Fatalf("got %+v, %v", p2, err)
	}
	got, _ = s.GetTenantSSOPolicy(ctx, org.ID)
	if len(got.Bindings) != 1 || got.Bindings[0].ConnectionID != connB || !got.Bindings[0].Active || got.Domains == nil || len(got.Domains) != 0 {
		t.Fatalf("got %+v", got)
	}
	if candidates, _ := s.ListAutoJoinCandidatesByDomain(ctx, "hooli.example", ""); len(candidates) != 0 {
		t.Fatalf("no longer a candidate, got %+v", candidates)
	}

	// No bindings keeps the row: the tenant is OFF.
	if _, err := putPolicy(client, s, &types.TenantSSOPolicy{TenantID: org.ID, Enforcement: types.EnforcementOptional}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || got.Bindings == nil || len(got.Bindings) != 0 || got.EffectiveEnforcement() != types.EnforcementOff {
		t.Fatalf("got %+v, %v", got, err)
	}

	// Outside a transaction the lock would end with its statement.
	if _, _, err := s.LockTenantSSOPolicy(ctx, org.ID); !errors.Is(err, ErrNoTransaction) {
		t.Fatalf("want ErrNoTransaction, got %v", err)
	}

	// Unknown tenant: no row to lock.
	if _, err := putPolicy(client, s, &types.TenantSSOPolicy{TenantID: newID(t), Enforcement: types.EnforcementOptional}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// Taking the lock writes nothing.
	bare := newTenant(t, s, "Bare", true)
	err = client.WithTx(ctx, func(ctx context.Context) error {
		_, locked, err := s.LockTenantSSOPolicy(ctx, bare.ID)
		if err == nil && !unwritten(locked) {
			err = fmt.Errorf("want the policy of a tenant that never wrote one, got %+v", locked)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetTenantSSOPolicy(ctx, bare.ID); err != nil || !unwritten(got) {
		t.Fatalf("nothing was written: %+v, %v", got, err)
	}

	// Deleting the tenant takes the policy with it.
	if err := s.DeleteTenant(ctx, org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTenantSSOPolicy(ctx, org.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

// Two requests change different parts of one tenant's policy at the same
// time: the second waits at the policy's lock, then sees what the first one
// committed, and neither undoes the other.
func TestIntegration_SSOPolicyConcurrentWriters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()
	org := newTenant(t, s, "Hooli", true)
	conn := newID(t)

	// The first request takes the lock, and holds it.
	held, release, first := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		first <- client.WithTx(ctx, func(ctx context.Context) error {
			if _, _, err := s.LockTenantSSOPolicy(ctx, org.ID); err != nil {
				return err
			}
			close(held)
			<-release
			return s.UpdateTenantSSODomains(ctx, org.ID, []string{"hooli.example"})
		})
	}()
	select {
	case <-held:
	case err := <-first:
		t.Fatalf("the first request did not get the lock: %v", err)
	}

	// The second one waits there.
	var seen *types.TenantSSOPolicy
	second := make(chan error, 1)
	go func() {
		second <- client.WithTx(ctx, func(ctx context.Context) error {
			_, locked, err := s.LockTenantSSOPolicy(ctx, org.ID)
			if err != nil {
				return err
			}
			seen = locked
			return s.UpdateTenantSSOPolicy(ctx, org.ID, types.EnforcementRequired, true, []types.SSOBinding{{ConnectionID: conn, Active: true}})
		})
	}()
	select {
	case err := <-second:
		t.Fatalf("the second request did not wait for the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	if err := <-first; err != nil {
		t.Fatalf("the first request: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("the second request: %v", err)
	}
	if seen == nil || len(seen.Domains) != 1 || seen.Domains[0] != "hooli.example" {
		t.Fatalf("with the lock, the second request sees what the first one committed: %+v", seen)
	}

	got, err := s.GetTenantSSOPolicy(ctx, org.ID)
	if err != nil || len(got.Domains) != 1 || len(got.Bindings) != 1 || !got.AutoJoin || !got.RequiresSSO() {
		t.Fatalf("both changes are stored: %+v, %v", got, err)
	}
}

func TestIntegration_SSOPolicyConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()
	acme, globex := newTenant(t, s, "Acme", true), newTenant(t, s, "Globex", true)
	conn := newID(t)

	// Auto-join only with REQUIRED.
	_, err := putPolicy(client, s, &types.TenantSSOPolicy{
		TenantID: acme.ID, Enforcement: types.EnforcementOptional, AutoJoin: true,
		Domains:  []string{"acme.example"},
		Bindings: []types.SSOBinding{{ConnectionID: conn, Active: true}},
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgErrCodeCheckViolation {
		t.Fatalf("want a check violation, got %v", err)
	}

	// A connection is bound by one tenant only.
	if _, err := putPolicy(client, s, &types.TenantSSOPolicy{
		TenantID: acme.ID, Enforcement: types.EnforcementOptional,
		Bindings: []types.SSOBinding{{ConnectionID: conn, Active: true}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := putPolicy(client, s, &types.TenantSSOPolicy{
		TenantID: globex.ID, Enforcement: types.EnforcementOptional,
		Bindings: []types.SSOBinding{{ConnectionID: conn}},
	}); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("want ErrDuplicateKey, got %v", err)
	}
	// The refused write left nothing behind: its transaction was rolled back.
	if got, err := s.GetTenantSSOPolicy(ctx, globex.ID); err != nil || !unwritten(got) {
		t.Fatalf("nothing was written: %+v, %v", got, err)
	}

	// The tenant that binds it may write the same binding again: it is replaced, not added.
	if p, err := putPolicy(client, s, &types.TenantSSOPolicy{
		TenantID: acme.ID, Enforcement: types.EnforcementOptional,
		Bindings: []types.SSOBinding{{ConnectionID: conn}},
	}); err != nil || len(p.Bindings) != 1 || p.Bindings[0].Active {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestIntegration_MFARequirement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, _ := newIntegrationStorage(t)
	ctx := context.Background()

	org := newTenant(t, s, "Corp", true)

	before, err := s.UpdateTenantMFARequirement(ctx, org.ID, types.MFARequirementNone)
	if err != nil || before != types.MFARequirementNone {
		t.Fatalf("got %q, %v", before, err)
	}
	before, err = s.UpdateTenantMFARequirement(ctx, org.ID, types.MFARequirementRequired)
	if err != nil || before != types.MFARequirementNone {
		t.Fatalf("got %q, %v", before, err)
	}
	got, _ := s.GetTenantByID(ctx, org.ID)
	if got.MFARequirement != types.MFARequirementRequired {
		t.Fatalf("got %+v", got)
	}
	before, err = s.UpdateTenantMFARequirement(ctx, org.ID, types.MFARequirementNone)
	if err != nil || before != types.MFARequirementRequired {
		t.Fatalf("got %q, %v", before, err)
	}

	if _, err := s.UpdateTenantMFARequirement(ctx, newID(t), types.MFARequirementNone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := s.UpdateTenantMFARequirement(ctx, org.ID, "totp"); err == nil {
		t.Fatal("the check constraint must refuse an unknown value")
	}
}

func TestIntegration_Members(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()

	org := newTenant(t, s, "Initech", true)
	account := newID(t)

	// Adding a member twice in one transaction: the second is ErrDuplicateKey,
	// and the transaction goes on and commits what else it wrote.
	err := client.WithTx(ctx, func(ctx context.Context) error {
		if _, err := s.AddMember(ctx, org.ID, account); err != nil {
			return err
		}
		if _, err := s.AddMember(ctx, org.ID, account); !errors.Is(err, ErrDuplicateKey) {
			return fmt.Errorf("want ErrDuplicateKey, got %v", err)
		}
		return s.AddInvitation(ctx, org.ID, "iris@test.example", time.Hour)
	})
	if err != nil {
		t.Fatal(err)
	}
	if members, _, err := s.ListMembersByTenantID(ctx, org.ID); err != nil || len(members) != 1 {
		t.Fatalf("one membership, got %+v, %v", members, err)
	}
	if found, err := s.HasInvitationByTenantAndEmail(ctx, org.ID, "iris@test.example"); err != nil || !found {
		t.Fatalf("written after the duplicate, in the same transaction: %v, %v", found, err)
	}

	if _, err := s.AddMember(ctx, newID(t), account); !errors.Is(err, ErrForeignKeyViolation) {
		t.Fatalf("want ErrForeignKeyViolation, got %v", err)
	}

	if err := s.DeleteMember(ctx, org.ID, newID(t)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.DeleteMember(ctx, org.ID, account); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMember(ctx, org.ID, account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestIntegration_PendingInvitations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()
	const email = "iris@test.example"

	initech := newTenant(t, s, "Initech", true)
	disabled := newTenant(t, s, "Disabled", false)
	expired := newTenant(t, s, "Expired", true)

	if found, err := s.HasInvitationByTenantAndEmail(ctx, initech.ID, email); err != nil || found {
		t.Fatalf("got %v, %v", found, err)
	}
	// The caller lower-cases the address: the table refuses anything else.
	var pgErr *pgconn.PgError
	if err := s.AddInvitation(ctx, initech.ID, "Iris@Test.example", time.Hour); !errors.As(err, &pgErr) || pgErr.Code != pgErrCodeCheckViolation {
		t.Fatalf("want a check violation, got %v", err)
	}
	// An invitation that has run out is renewed by a repeat.
	if err := s.AddInvitation(ctx, initech.ID, email, -time.Minute); err != nil {
		t.Fatal(err)
	}
	if found, err := s.HasInvitationByTenantAndEmail(ctx, initech.ID, email); err != nil || found {
		t.Fatalf("an expired invitation admits nothing: %v, %v", found, err)
	}
	if err := s.AddInvitation(ctx, initech.ID, email, time.Hour); err != nil {
		t.Fatal(err)
	}
	if found, err := s.HasInvitationByTenantAndEmail(ctx, initech.ID, email); err != nil || !found {
		t.Fatalf("got %v, %v", found, err)
	}
	if err := s.AddInvitation(ctx, disabled.ID, email, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.AddInvitation(ctx, expired.ID, email, -time.Minute); err != nil {
		t.Fatal(err)
	}

	// Enabled tenants only, unexpired invitations only, minus memberships.
	tenants, err := s.ListInvitedTenantsByEmail(ctx, email, "test.example", "")
	if err != nil || len(tenants) != 1 || tenants[0].ID != initech.ID || !tenants[0].Invited || tenants[0].AutoJoinCandidate {
		t.Fatalf("got %+v, %v", tenants, err)
	}

	// A tenant that requires SSO and has domains admits only the addresses in
	// them, whenever it was invited.
	required := newTenant(t, s, "Required", true)
	if err := s.AddInvitation(ctx, required.ID, email, time.Hour); err != nil {
		t.Fatal(err)
	}
	policy := &types.TenantSSOPolicy{
		TenantID: required.ID, Enforcement: types.EnforcementRequired,
		Domains:  []string{"elsewhere.example"},
		Bindings: []types.SSOBinding{{ConnectionID: newID(t), Active: true}},
	}
	if _, err := putPolicy(client, s, policy); err != nil {
		t.Fatal(err)
	}
	if tenants, err = s.ListInvitedTenantsByEmail(ctx, email, "test.example", ""); err != nil || len(tenants) != 1 || tenants[0].ID != initech.ID {
		t.Fatalf("an address outside the domains of a tenant that requires SSO is not listed: %+v, %v", tenants, err)
	}
	policy.Domains = []string{"elsewhere.example", "test.example"}
	if _, err := putPolicy(client, s, policy); err != nil {
		t.Fatal(err)
	}
	if tenants, err = s.ListInvitedTenantsByEmail(ctx, email, "test.example", ""); err != nil || len(tenants) != 2 {
		t.Fatalf("an address in the domains is listed: %+v, %v", tenants, err)
	}
	if err := s.DeleteInvitation(ctx, required.ID, email); err != nil {
		t.Fatal(err)
	}

	account := newID(t)
	if _, err := s.AddMember(ctx, initech.ID, account); err != nil {
		t.Fatal(err)
	}
	if tenants, err = s.ListInvitedTenantsByEmail(ctx, email, "test.example", account); err != nil || len(tenants) != 0 {
		t.Fatalf("a member is not invited again: %+v, %v", tenants, err)
	}

	if err := s.DeleteInvitation(ctx, initech.ID, email); err != nil {
		t.Fatal(err)
	}
	if found, err := s.HasInvitationByTenantAndEmail(ctx, initech.ID, email); err != nil || found {
		t.Fatalf("deleted: %v, %v", found, err)
	}
	if tenants, err = s.ListInvitedTenantsByEmail(ctx, email, "test.example", ""); err != nil || len(tenants) != 0 {
		t.Fatalf("the ones left are to a disabled tenant and expired: %+v, %v", tenants, err)
	}
}

func TestIntegration_ConnectionTimeouts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, client := newIntegrationStorage(t)
	ctx := context.Background()

	// What the server reports for each setting, on a connection of the pool.
	for name, want := range map[string]string{
		"lock_timeout":                        "2s",
		"statement_timeout":                   "5s",
		"idle_in_transaction_session_timeout": "15s",
	} {
		var got string
		err := client.Statement(ctx).
			Select().
			Column(sq.Expr("current_setting(?)", name)).
			QueryRowContext(ctx).
			Scan(&got)
		if err != nil || got != want {
			t.Errorf("%s: want %s, got %s, %v", name, want, got, err)
		}
	}
}

// setLocal sets a run-time parameter for the transaction of ctx.
func setLocal(ctx context.Context, client *db.DBClient, name, value string) error {
	var set string
	return client.Statement(ctx).
		Select().
		Column(sq.Expr("set_config(?, ?, TRUE)", name, value)).
		QueryRowContext(ctx).
		Scan(&set)
}

// holdTenant locks the tenant's row in a transaction that stays open until
// the returned function is called.
func holdTenant(t *testing.T, client *db.DBClient, s *Storage, tenantID string) (release func()) {
	t.Helper()

	locked, done, holder := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holder <- client.WithTx(context.Background(), func(ctx context.Context) error {
			if _, _, err := s.LockTenantSSOPolicy(ctx, tenantID); err != nil {
				return err
			}
			close(locked)
			<-done
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-holder:
		t.Fatalf("the first transaction did not get to hold the row: %v", err)
	}

	return func() {
		close(done)
		if err := <-holder; err != nil {
			t.Fatalf("the first transaction: %v", err)
		}
	}
}

// A write that finds its row locked by another transaction gives up after
// the lock timeout instead of waiting for that transaction to end, and the
// error is recognised through the storage's own wrapping.
func TestIntegration_LockTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()
	org := newTenant(t, s, "Hooli", true)
	release := holdTenant(t, client, s, org.ID)

	const lockTimeout = 300 * time.Millisecond
	start := time.Now()
	err := client.WithTx(ctx, func(ctx context.Context) error {
		if err := setLocal(ctx, client, "lock_timeout", "300ms"); err != nil {
			return err
		}
		_, _, err := s.LockTenantSSOPolicy(ctx, org.ID)
		return err
	})
	waited := time.Since(start)
	release()

	if !IsLockTimeout(err) || IsStatementTimeout(err) {
		t.Fatalf("want a lock timeout, got %v", err)
	}
	if waited < lockTimeout {
		t.Fatalf("want to wait at least %s, waited %s", lockTimeout, waited)
	}
}

// A statement that runs longer than the statement timeout is cancelled: here
// one that waits for a row another transaction holds, with no lock timeout.
func TestIntegration_StatementTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	s, client := newIntegrationStorage(t)
	ctx := context.Background()
	org := newTenant(t, s, "Hooli", true)
	release := holdTenant(t, client, s, org.ID)

	err := client.WithTx(ctx, func(ctx context.Context) error {
		if err := setLocal(ctx, client, "lock_timeout", "0"); err != nil {
			return err
		}
		if err := setLocal(ctx, client, "statement_timeout", "300ms"); err != nil {
			return err
		}
		_, err := s.UpdateTenantMFARequirement(ctx, org.ID, types.MFARequirementRequired)
		return err
	})
	release()

	if !IsStatementTimeout(err) || IsLockTimeout(err) {
		t.Fatalf("want a statement timeout, got %v", err)
	}
	if got, err := s.GetTenantByID(ctx, org.ID); err != nil || got.MFARequirement != types.MFARequirementNone {
		t.Fatalf("nothing was written: %+v, %v", got, err)
	}
}

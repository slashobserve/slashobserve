package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const fileKeyName = "From file"

type accountAction string

const (
	accountCreate accountAction = "created"
	accountUpdate accountAction = "updated"
	accountKeep   accountAction = "unchanged"
)

type keyAction string

const (
	keyAdd     keyAction = "added"
	keyReplace keyAction = "replaced"
	keyKeep    keyAction = "unchanged"
)

type provisionPlan struct {
	Account accountAction
	Key     keyAction
}

type provisionedState struct {
	Exists      bool
	Provisioned bool
	Role        Role
	Description string
	KeyHash     []byte
}

type provisionTarget struct {
	Role        Role
	Description string
	KeyHash     []byte
}

type ProvisionRefusedError struct {
	Name   string
	Reason string
}

func (e *ProvisionRefusedError) Error() string {
	return fmt.Sprintf("service account %q: %s", e.Name, e.Reason)
}

const (
	reasonNotProvisioned = "an account with this name was not created from the service-accounts file and is never taken over by it; give the file's account another name"
	reasonKeyReused      = "the key in its keyFile was used before; write a new key to the file"
	reasonUnknownRole    = "role is not a known role"
)

var errNotProvisioned = errors.New("account not provisioned")

func planProvision(current provisionedState, want provisionTarget) (provisionPlan, error) {
	if !current.Exists {
		return provisionPlan{Account: accountCreate, Key: keyAdd}, nil
	}
	if !current.Provisioned {
		return provisionPlan{}, errNotProvisioned
	}
	plan := provisionPlan{Account: accountKeep, Key: keyKeep}
	if current.Role != want.Role || current.Description != want.Description {
		plan.Account = accountUpdate
	}
	switch {
	case current.KeyHash == nil:
		plan.Key = keyAdd
	case !bytes.Equal(current.KeyHash, want.KeyHash):
		plan.Key = keyReplace
	}
	return plan, nil
}

func (s *Service) EnsureProvisioned(ctx context.Context, accounts []config.ServiceAccount) error {
	for _, a := range accounts {
		if err := s.ensureProvisioned(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureProvisioned(ctx context.Context, a config.ServiceAccount) error {
	role, ok := ParseRole(a.Role)
	if !ok {
		return &ProvisionRefusedError{Name: a.Name, Reason: reasonUnknownRole}
	}
	token := a.Key.Reveal()
	want := provisionTarget{Role: role, Description: a.Description, KeyHash: hashToken(token)}
	current, accountID, err := s.provisionedState(ctx, a.Name)
	if err != nil {
		return fmt.Errorf("read service account: %w", err)
	}
	plan, err := planProvision(current, want)
	if errors.Is(err, errNotProvisioned) {
		return &ProvisionRefusedError{Name: a.Name, Reason: reasonNotProvisioned}
	}
	if plan.Account != accountKeep {
		row, err := s.queries.UpsertProvisionedServiceAccount(ctx, dbsqlc.UpsertProvisionedServiceAccountParams{
			Name: a.Name, Description: a.Description, Role: string(role),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return &ProvisionRefusedError{Name: a.Name, Reason: reasonNotProvisioned}
		}
		if err != nil {
			return fmt.Errorf("write service account: %w", err)
		}
		accountID = row.ID
	}
	if plan.Key != keyKeep {
		err := s.queries.ReplaceProvisionedKey(ctx, dbsqlc.ReplaceProvisionedKeyParams{
			ServiceAccountID: accountID,
			Name:             fileKeyName,
			DisplayPrefix:    token[:displayPrefixLength],
			TokenHash:        want.KeyHash,
		})
		if isUniqueViolation(err) {
			return &ProvisionRefusedError{Name: a.Name, Reason: reasonKeyReused}
		}
		if err != nil {
			return fmt.Errorf("write service account key: %w", err)
		}
	}
	s.logger.InfoContext(ctx, "service account provisioned", slog.String("service_account", a.Name),
		slog.String("account", string(plan.Account)), slog.String("key", string(plan.Key)), slog.String("role", string(role)))
	return nil
}

func (s *Service) provisionedState(ctx context.Context, name string) (provisionedState, pgtype.UUID, error) {
	account, err := s.queries.GetServiceAccountByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return provisionedState{}, pgtype.UUID{}, nil
	}
	if err != nil {
		return provisionedState{}, pgtype.UUID{}, err
	}
	state := provisionedState{Exists: true, Provisioned: account.Provisioned, Role: Role(account.Role), Description: account.Description}
	key, err := s.queries.ActiveKeyByName(ctx, dbsqlc.ActiveKeyByNameParams{ServiceAccountID: account.ID, Name: fileKeyName})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return provisionedState{}, pgtype.UUID{}, err
	default:
		state.KeyHash = key.TokenHash
	}
	return state, account.ID, nil
}

package accessrequests

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func canCommentNow(r dbsqlc.AccessRequestsToSettleRow) bool {
	facts := folders.Facts{Restricted: folders.Restricted(r.AdminsOnly, r.Listed), AdminsOnly: r.AdminsOnly, Grants: levelsOf(r.Grants), Creator: r.Creator, Personal: r.Personal, Owner: r.Owner}
	return folders.AbilitiesFor(folders.LevelFor(roleOf(r.Role), facts)).Comment
}

func (s *Service) SettleFolder(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, folder pgtype.UUID) error {
	return settle(ctx, q, by, dbsqlc.AccessRequestsToSettleParams{FolderID: folder})
}

func (s *Service) SettleRequester(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, kind auth.PrincipalKind, id pgtype.UUID) error {
	return settle(ctx, q, by, dbsqlc.AccessRequestsToSettleParams{RequesterKind: string(kind), RequesterID: id})
}

func settle(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, params dbsqlc.AccessRequestsToSettleParams) error {
	rows, err := q.AccessRequestsToSettle(ctx, params)
	if err != nil {
		return fmt.Errorf("list access requests to settle: %w", err)
	}
	grantedBy, _ := db.ParseUUID(by.ID)
	for _, r := range rows {
		if !canCommentNow(r) {
			continue
		}
		n, err := q.GrantAccessRequest(ctx, dbsqlc.GrantAccessRequestParams{ID: r.ID, GrantedByKind: string(by.Kind), GrantedByID: grantedBy})
		if err != nil {
			return fmt.Errorf("grant access request: %w", err)
		}
		if n == 0 {
			continue
		}
		obj := Object{Kind: KindDashboard, ID: r.DashboardID}
		if r.ObservationID.Valid {
			obj = Object{Kind: KindObservation, ID: r.ObservationID}
		}
		_, err = activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeAccessRequestGranted,
			TargetID: r.ID,
			Owner:    ownerOf(obj),
			Actor:    activity.By(ctx, by),
			Data: grantedData{
				Object:      objectData{Kind: obj.Kind, ID: obj.ID.String()},
				Title:       r.Title,
				RequestedBy: requesterData{Kind: r.RequesterKind, ID: r.RequesterID.String(), Name: r.RequesterName},
			},
		}, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

type ChangeLog struct {
	auth.ChangeLog
	Requests *Service
}

func (c ChangeLog) RoleChanged(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, change auth.RoleChange) error {
	if err := c.ChangeLog.RoleChanged(ctx, q, by, change); err != nil {
		return err
	}
	return c.Requests.SettleRequester(ctx, q, by, auth.KindUser, change.UserID)
}

func (c ChangeLog) ServiceAccountUpdated(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, before, after dbsqlc.ServiceAccount) error {
	if err := c.ChangeLog.ServiceAccountUpdated(ctx, q, by, before, after); err != nil {
		return err
	}
	if before.Role == after.Role {
		return nil
	}
	return c.Requests.SettleRequester(ctx, q, by, auth.KindServiceAccount, after.ID)
}

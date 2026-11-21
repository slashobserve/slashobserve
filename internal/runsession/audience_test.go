package runsession

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestEachKindOfRunReadsAdminsOnlyDataOnlyWhenEveryoneWhoReadsWhatItWritesIsAnAdmin(t *testing.T) {
	cases := []struct {
		name string
		row  dbsqlc.GetRunWritesRow
		want datasource.Audience
	}{
		{"a private chat ask with no subject", dbsqlc.GetRunWritesRow{Intent: "ask"}, datasource.AudienceCaller},
		{"a private chat ask about a dashboard", dbsqlc.GetRunWritesRow{Intent: "ask", OnDashboard: true}, datasource.AudienceCaller},
		{"a private chat ask about an observation", dbsqlc.GetRunWritesRow{Intent: "ask"}, datasource.AudienceCaller},
		{"a thread ask on a dashboard in a shared folder", dbsqlc.GetRunWritesRow{Intent: "ask", InThread: true, OnDashboard: true}, datasource.AudienceEveryone},
		{"a thread ask on a dashboard in a folder only Admins read", dbsqlc.GetRunWritesRow{Intent: "ask", InThread: true, ThreadAdminsOnly: true, OnDashboard: true}, datasource.AudienceAdmins},
		{"a thread ask on an observation in a shared folder", dbsqlc.GetRunWritesRow{Intent: "ask", InThread: true, IntoObservation: true}, datasource.AudienceEveryone},
		{"a thread ask on an observation in a folder only Admins read", dbsqlc.GetRunWritesRow{Intent: "ask", InThread: true, ThreadAdminsOnly: true, IntoObservation: true, ObservationAdminsOnly: true}, datasource.AudienceAdmins},
		{"a build before its first save", dbsqlc.GetRunWritesRow{Intent: "apply"}, datasource.AudienceCaller},
		{"a build saved into a shared folder", dbsqlc.GetRunWritesRow{Intent: "apply", Building: true}, datasource.AudienceEveryone},
		{"a build saved into a folder only Admins read", dbsqlc.GetRunWritesRow{Intent: "apply", Building: true, BuildAdminsOnly: true}, datasource.AudienceAdmins},
		{"a Home question that started a build in a shared folder", dbsqlc.GetRunWritesRow{Intent: "ask", Building: true}, datasource.AudienceEveryone},
		{"an investigation filing into an observation in a shared folder", dbsqlc.GetRunWritesRow{Intent: "ask", IntoObservation: true, OnDashboard: true}, datasource.AudienceEveryone},
		{"an investigation filing into an observation only Admins read", dbsqlc.GetRunWritesRow{Intent: "ask", IntoObservation: true, ObservationAdminsOnly: true}, datasource.AudienceAdmins},
		{"a revision of a dashboard in a shared folder", dbsqlc.GetRunWritesRow{Intent: "apply", OnDashboard: true}, datasource.AudienceEveryone},
		{"a revision of a dashboard only Admins read", dbsqlc.GetRunWritesRow{Intent: "apply", OnDashboard: true, DashboardAdminsOnly: true}, datasource.AudienceAdmins},
		{"a revision with a build in a folder only Admins read", dbsqlc.GetRunWritesRow{Intent: "apply", OnDashboard: true, DashboardAdminsOnly: true, Building: true, BuildAdminsOnly: true}, datasource.AudienceAdmins},
		{"an apply asked in a thread only Admins read on a dashboard others read", dbsqlc.GetRunWritesRow{Intent: "apply", InThread: true, ThreadAdminsOnly: true, OnDashboard: true}, datasource.AudienceEveryone},
		{"a suggestion on a dashboard in a shared folder", dbsqlc.GetRunWritesRow{Intent: "suggest", OnDashboard: true}, datasource.AudienceEveryone},
		{"a suggestion on a dashboard only Admins read", dbsqlc.GetRunWritesRow{Intent: "suggest", OnDashboard: true, DashboardAdminsOnly: true}, datasource.AudienceAdmins},
		{"a shared chat turn in a thread only Admins read", dbsqlc.GetRunWritesRow{Intent: "ask", InThread: true, ThreadAdminsOnly: true, OnDashboard: true}, datasource.AudienceAdmins},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tools.RunAudience(writesOf(tc.row)); got != tc.want {
				t.Fatalf("audience = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTheFoldersARunWritesToAreThoseOfWhatItWritesNotOfWhatItAsksAbout(t *testing.T) {
	thread, dashboard, build := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	ask := writesOf(dbsqlc.GetRunWritesRow{Intent: "ask", InThread: true, ThreadFolderID: thread, OnDashboard: true, DashboardFolderID: dashboard})
	if !slices.Equal(ask.Folders, []pgtype.UUID{thread}) {
		t.Fatalf("an ask in a thread writes to %v, want only the thread's folder", ask.Folders)
	}
	apply := writesOf(dbsqlc.GetRunWritesRow{Intent: "apply", OnDashboard: true, DashboardFolderID: dashboard, Building: true, BuildFolderID: dashboard})
	if !slices.Equal(apply.Folders, []pgtype.UUID{dashboard}) {
		t.Fatalf("a revision writes to %v, want the dashboard's folder once", apply.Folders)
	}
	create := writesOf(dbsqlc.GetRunWritesRow{Intent: "ask", Building: true, BuildFolderID: build})
	if !slices.Equal(create.Folders, []pgtype.UUID{build}) {
		t.Fatalf("a new dashboard writes to %v, want the build's folder", create.Folders)
	}
}

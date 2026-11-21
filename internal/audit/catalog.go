package audit

import (
	"maps"
	"slices"
)

type Action string

type Category string

const (
	CategoryAuth         Category = "auth"
	CategoryAccess       Category = "access"
	CategoryContent      Category = "content"
	CategoryConversation Category = "conversation"
	CategoryAgent        Category = "agent"
	CategoryQuery        Category = "query"
	CategoryAdmin        Category = "admin"
	CategorySystem       Category = "system"
)

type Class string

const (
	ClassChange Class = "change"
	ClassAccess Class = "access"
)

type Spec struct {
	Category Category
	Class    Class
	Data     []string
}

const (
	ActionGap    Action = "audit.gap"
	ActionPrune  Action = "audit.prune"
	ActionRefuse Action = "request.refuse"
)

var (
	titleKeys    = []string{"title"}
	movedKeys    = []string{"title", "from_folder_id", "to_folder_id"}
	versionKeys  = []string{"version", "parent_version", "restored_from", "title", "summary"}
	annotKeys    = []string{"title", "panel_id", "tags", "time_from", "time_to"}
	savedKeys    = []string{"title", "kind", "changed", "from_folder_id", "to_folder_id"}
	obsKeys      = []string{"title", "from_title", "from_folder_id", "to_folder_id", "revision"}
	blockKeys    = []string{"title", "block_id", "block_kind", "state"}
	proposalKeys = []string{"title", "revision", "base_version", "version"}
	threadKeys   = []string{"object", "title", "version", "panel_id", "message_id", "resolved_by_version", "mentioned"}
	runKeys      = []string{"source", "continues_id", "attempt", "result_version", "failure_code", "cost_usd", "dashboard_title", "panels_repaired", "result"}
	linkKeys     = []string{"object", "title", "prefix", "expires_at"}
	accessKeys   = []string{"object", "title", "general", "requested_by"}
	accountKeys  = []string{"name", "role", "takes_runs", "key_id", "key_name", "expires_at"}
	personKeys   = []string{"name", "email", "role", "from_role", "to_role", "manual", "source", "sessions", "grants"}
	webhookKeys  = []string{"name", "host", "event_types", "changes", "url_changed"}
	grantKeys    = []string{"client", "person_id", "person_name", "mode", "reason"}
	domainKeys   = []string{"domain"}
	grafanaKeys  = []string{"name", "host", "changes", "token_changed"}
	sourceKeys   = []string{"uid", "type", "name", "host", "changes", "address_changed", "settings_changed", "credentials_changed"}
	audienceKeys = []string{"uid", "type", "name", "host", "from", "to"}
	gapKeys      = []string{"lost"}
	pruneKeys    = []string{"partitions", "before"}
	jobKeys      = []string{"trigger", "duration_ms"}
	importKeys   = []string{"connection_id"}

	renamedKeys    = []string{"from_title", "title"}
	permissionKeys = []string{"added", "removed", "changed", "count_before", "count_after", "truncated"}
	memberKeys     = []string{"user_id", "user_name"}
)

func change(c Category, data ...[]string) Spec {
	return Spec{Category: c, Class: ClassChange, Data: slices.Concat(data...)}
}

func access(c Category, data ...[]string) Spec {
	return Spec{Category: c, Class: ClassAccess, Data: slices.Concat(data...)}
}

var catalog = map[Action]Spec{
	"auth.login":               change(CategoryAuth),
	"auth.logout":              change(CategoryAuth),
	"oauth.client_register":    change(CategoryAuth),
	"oauth.authorize":          change(CategoryAuth),
	"oauth.token":              change(CategoryAuth),
	"oauth.revoke":             change(CategoryAuth),
	"auth.login_failed":        access(CategoryAuth),
	"auth.credential_rejected": access(CategoryAuth),
	"auth.csrf_rejected":       access(CategoryAuth),
	"request.refuse":           access(CategoryAccess),

	"dashboard.view":   access(CategoryAccess),
	"observation.view": access(CategoryAccess),
	"public_link.view": access(CategoryAccess),
	"audit.search":     access(CategoryAccess),
	"audit.export":     access(CategoryAccess),

	"dashboard.create":          change(CategoryContent, versionKeys),
	"dashboard.update":          change(CategoryContent, versionKeys),
	"dashboard.restore":         change(CategoryContent, versionKeys),
	"dashboard.move":            change(CategoryContent, movedKeys),
	"dashboard.delete":          change(CategoryContent, titleKeys),
	"annotation.create":         change(CategoryContent, annotKeys),
	"annotation.update":         change(CategoryContent, annotKeys),
	"annotation.delete":         change(CategoryContent, annotKeys),
	"finding.create":            change(CategoryContent, titleKeys, []string{"agent_request_id"}),
	"finding.pin":               change(CategoryContent, []string{"annotation_id"}),
	"finding.move":              change(CategoryContent, movedKeys),
	"saved_search.create":       change(CategoryContent, savedKeys),
	"saved_search.update":       change(CategoryContent, savedKeys),
	"saved_search.move":         change(CategoryContent, savedKeys),
	"saved_search.delete":       change(CategoryContent, savedKeys),
	"observation.create":        change(CategoryContent, obsKeys),
	"observation.update":        change(CategoryContent, obsKeys),
	"observation.move":          change(CategoryContent, obsKeys),
	"observation.delete":        change(CategoryContent, obsKeys),
	"observation.resolve":       change(CategoryContent, obsKeys),
	"observation.reopen":        change(CategoryContent, obsKeys),
	"observation.publish":       change(CategoryContent, obsKeys),
	"observation.rca_withdraw":  change(CategoryContent, obsKeys),
	"observation.block_add":     change(CategoryContent, blockKeys),
	"observation.block_update":  change(CategoryContent, blockKeys),
	"observation.block_keep":    change(CategoryContent, blockKeys),
	"observation.block_discard": change(CategoryContent, blockKeys),
	"observation.block_remove":  change(CategoryContent, blockKeys),
	"observation.arrange":       change(CategoryContent),
	"proposal.open":             change(CategoryContent, proposalKeys),
	"proposal.update":           change(CategoryContent, proposalKeys),
	"proposal.merge":            change(CategoryContent, proposalKeys),
	"proposal.discard":          change(CategoryContent, proposalKeys),
	"folder.create":             change(CategoryContent),
	"folder.rename":             change(CategoryContent, renamedKeys),
	"folder.delete":             change(CategoryContent),
	"grafana_import.start":      change(CategoryContent, importKeys),
	"grafana_import.cancel":     change(CategoryContent, importKeys),

	"thread.open":                 change(CategoryConversation, threadKeys),
	"thread.message":              change(CategoryConversation, threadKeys),
	"thread.message_edit":         change(CategoryConversation, threadKeys),
	"thread.message_delete":       change(CategoryConversation, threadKeys),
	"thread.resolve":              change(CategoryConversation, threadKeys),
	"thread.reopen":               change(CategoryConversation, threadKeys),
	"thread.mention":              change(CategoryConversation, threadKeys),
	"thread.agent_set":            change(CategoryConversation),
	"chat.create":                 change(CategoryConversation),
	"chat.turn":                   change(CategoryConversation),
	"chat.delete":                 change(CategoryConversation),
	"run.share":                   change(CategoryConversation),
	"run.withdraw":                change(CategoryConversation),
	"finding.apply":               change(CategoryConversation),
	"finding.save_as_observation": change(CategoryConversation),

	"run.queue":       change(CategoryAgent, runKeys),
	"run.start":       change(CategoryAgent, runKeys),
	"run.finish":      change(CategoryAgent, runKeys),
	"run.cancel":      change(CategoryAgent, runKeys),
	"run.release":     change(CategoryAgent, runKeys),
	"run.keep":        change(CategoryAgent, runKeys),
	"run.retry":       change(CategoryAgent),
	"run.continue":    change(CategoryAgent),
	"run.update":      change(CategoryAgent),
	"run.archive":     change(CategoryAgent),
	"agent.tool_call": access(CategoryAgent),

	"query.run": access(CategoryQuery),

	"user.role_change":           change(CategoryAdmin, personKeys),
	"user.sign_out_everywhere":   change(CategoryAdmin, personKeys),
	"user.invite":                change(CategoryAdmin, personKeys),
	"user.invite_update":         change(CategoryAdmin, personKeys),
	"user.invite_withdraw":       change(CategoryAdmin, personKeys),
	"user.deactivate":            change(CategoryAdmin, personKeys),
	"user.reactivate":            change(CategoryAdmin, personKeys),
	"user.remove":                change(CategoryAdmin, personKeys),
	"service_account.create":     change(CategoryAdmin, accountKeys),
	"service_account.update":     change(CategoryAdmin, accountKeys),
	"service_account.delete":     change(CategoryAdmin, accountKeys),
	"service_account.key_create": change(CategoryAdmin, accountKeys),
	"service_account.key_revoke": change(CategoryAdmin, accountKeys),
	"webhook.create":             change(CategoryAdmin, webhookKeys),
	"webhook.update":             change(CategoryAdmin, webhookKeys),
	"webhook.secret_rotate":      change(CategoryAdmin, webhookKeys),
	"webhook.delete":             change(CategoryAdmin, webhookKeys),
	"webhook.skip":               change(CategoryAdmin),
	"webhook.test":               change(CategoryAdmin),
	"mcp_client.connect":         change(CategoryAdmin, grantKeys),
	"mcp_client.disconnect":      change(CategoryAdmin, grantKeys),
	"mcp_client.update":          change(CategoryAdmin),
	"mcp_client.deny":            change(CategoryAdmin),
	"outbound_domain.add":        change(CategoryAdmin, domainKeys),
	"outbound_domain.remove":     change(CategoryAdmin, domainKeys),
	"grafana_connection.create":  change(CategoryAdmin, grafanaKeys),
	"grafana_connection.update":  change(CategoryAdmin, grafanaKeys),
	"grafana_connection.delete":  change(CategoryAdmin, grafanaKeys),
	"datasource.create":          change(CategoryAdmin, sourceKeys),
	"datasource.update":          change(CategoryAdmin, sourceKeys),
	"datasource.delete":          change(CategoryAdmin, sourceKeys),
	"datasource.audience_change": change(CategoryAdmin, audienceKeys),
	"public_link.create":         change(CategoryAdmin, linkKeys),
	"public_link.revoke":         change(CategoryAdmin, linkKeys),
	"access_request.open":        change(CategoryAdmin, accessKeys),
	"access_request.grant":       change(CategoryAdmin, accessKeys),
	"folder.permissions_change":  change(CategoryAdmin, permissionKeys),
	"team.create":                change(CategoryAdmin),
	"team.delete":                change(CategoryAdmin),
	"team.member_add":            change(CategoryAdmin, memberKeys),
	"team.member_remove":         change(CategoryAdmin, memberKeys),
	"action.run":                 change(CategoryAdmin),

	"server.start":    change(CategorySystem),
	"server.stop":     change(CategorySystem),
	"migration.apply": change(CategorySystem),
	"provision.apply": change(CategorySystem),
	"job.run":         change(CategorySystem, jobKeys),
	"webhook.deliver": access(CategorySystem),
	ActionPrune:       change(CategorySystem, pruneKeys),
	ActionGap:         change(CategorySystem, gapKeys),
}

func Lookup(a Action) (Spec, bool) {
	s, ok := catalog[a]
	return s, ok
}

func Actions() []Action {
	return slices.Sorted(maps.Keys(catalog))
}

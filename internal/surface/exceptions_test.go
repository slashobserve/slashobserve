package surface

type exception struct{ MCP, CLI string }

const (
	savedSearchCuration = "people delete saved searches; an agent saves, lists and updates them"
	builtInRuns         = "an outside agent does its own work over MCP; the built-in agent's runs are read in the web app and the CLI, and their events and cost reach MCP through list_activity"
	consentScreen       = "browser consent screen"
	connectedAgents     = "connected agents are a person's credentials; keys never manage credentials, the CLI signs in with a key, and no MCP tool manages a person's credentials"
	webhookAdmin        = "An Admin decides where workspace activity is sent; an agent that could register a webhook could send it anywhere."
	datasourceAdmin     = "An Admin decides where the server connects and with which credentials; an agent that could add or change a datasource, or read its address, could point the server anywhere. Agents list datasources and recheck them."
	contextSourceAdmin  = "An Admin decides which servers the server connects to and which of their tools an agent may call; an agent that could register a source or approve a tool could point the server anywhere and widen its own reach."
	publicLink          = "a public link is a person's decision: no agent or MCP tool creates, lists or revokes one"
	noMCPDeletes        = "no MCP tool deletes a dashboard, folder, annotation or block, so people delete in the web app, the HTTP API or the CLI"
	panelActions        = "runs a panel action a person clicks; the agent and MCP clients never add, change or run actions"
	browserSignIn       = "the browser's sign-in and session flow; keys and MCP clients authenticate with a bearer token"
	personOnly          = "a person's own action in the web app: keys are refused (auth.RequireUser or 403 user_session_required), and the CLI signs in with a key"
	openRecording       = "dashboard opens are recorded for people only; a key that posts one records nothing"
	accountKeys         = "keys never create, list or revoke keys (auth.RequireUser), and no MCP tool manages credentials"
	webRendering        = "the web app's rendering of a saved dashboard or panel; agents run queries with query_range and query_instant, and the CLI with observe logs and observe traces"
	liveWebPage         = "a live event stream for an open web page; MCP is stateless and does not stream"
	publicView          = "an anonymous read through a public link token; there is no principal"
	grafanaConnecting   = "a Grafana connection stores a credential, and connecting is a person's decision like connecting an MCP client: no agent, MCP tool or key creates, changes, tests or deletes one"
	endsSignIns         = "an Admin's decision that ends a person's sessions and connected agents; keys never manage them (auth.RequireUser), the CLI signs in with a key, and no MCP tool manages them"
	removesPeople       = "an Admin removes a deactivated person in the web app; keys never manage people's accounts (auth.RequireUser), and the CLI signs in with a key"
	peopleAdmin         = "who may sign in, and with which role, is an Admin's decision like a webhook; an Admin key reaches it through the CLI, and no MCP tool reads or manages people"
	viaApply            = "observe dashboards apply saves a spec file as a new dashboard or a new version through provision/apply, which is keyed by the file path, so a retried write does not duplicate"
	presenceReport      = "an open dashboard page reports itself every 20 seconds, for people only; agents and the CLI read who is looking with list_dashboard_viewers and observe dashboards viewers"
	auditFileExport     = "an audit export is a file of up to a million rows for a person to keep; an MCP answer holds at most 16 KiB, so agents page the same events with search_audit_events"
	notBuilt            = "no tool or command yet"
)

var exceptions = map[string]exception{
	"runAction": {MCP: panelActions, CLI: panelActions},

	"exportAuditEvents": {MCP: auditFileExport},

	"deleteAnnotation":     {MCP: noMCPDeletes},
	"deleteMessage":        {MCP: noMCPDeletes},
	"archiveChat":          {MCP: noMCPDeletes},
	"withdrawSharedAnswer": {MCP: noMCPDeletes},
	"markThreadRead":       {MCP: personOnly, CLI: personOnly},

	"getAssetImage":   {MCP: personOnly, CLI: personOnly},
	"getAssetGeojson": {MCP: personOnly, CLI: personOnly},

	"authLogin":    {MCP: browserSignIn, CLI: browserSignIn},
	"authCallback": {MCP: browserSignIn, CLI: browserSignIn},
	"authLogout":   {MCP: browserSignIn, CLI: browserSignIn},
	"authSession":  {MCP: browserSignIn, CLI: browserSignIn},
	"getMe":        {MCP: notBuilt},
	"getInstance":  {MCP: notBuilt},

	"createDashboard":        {CLI: viaApply},
	"listDashboardTags":      {MCP: notBuilt},
	"deleteDashboard":        {MCP: noMCPDeletes},
	"createDashboardVersion": {CLI: viaApply},
	"recordDashboardView":    {MCP: openRecording, CLI: openRecording},
	"reportDashboardViewer":  {MCP: presenceReport, CLI: presenceReport},

	"streamDashboardEvents": {MCP: liveWebPage, CLI: liveWebPage},

	"logVolume": {MCP: notBuilt},

	"listFindings":           {MCP: notBuilt, CLI: notBuilt},
	"getFinding":             {MCP: notBuilt, CLI: notBuilt},
	"pinFinding":             {MCP: notBuilt, CLI: notBuilt},
	"saveFindingAsDashboard": {MCP: notBuilt, CLI: notBuilt},
	"getFindingPanelData":    {MCP: personOnly, CLI: personOnly},

	"deleteObservation":      {MCP: noMCPDeletes},
	"withdrawRca":            {MCP: noMCPDeletes},
	"removeObservationBlock": {MCP: noMCPDeletes},
	"getSnapshot":            {MCP: webRendering, CLI: webRendering},
	"observationBlockData":   {MCP: webRendering, CLI: webRendering},
	"listRcaPublicLinks":     {MCP: publicLink, CLI: publicLink},
	"createRcaPublicLink":    {MCP: publicLink, CLI: publicLink},
	"deleteRcaPublicLink":    {MCP: publicLink, CLI: publicLink},
	"viewPublicRca":          {MCP: publicView, CLI: publicView},
	"viewPublicRcaSnapshot":  {MCP: publicView, CLI: publicView},

	"getFolder":            {MCP: notBuilt, CLI: notBuilt},
	"deleteFolder":         {MCP: noMCPDeletes},
	"getFolderPermissions": {MCP: notBuilt},
	"setFolderPermissions": {MCP: notBuilt},

	"listGrafanaConnections":  {MCP: notBuilt, CLI: notBuilt},
	"createGrafanaConnection": {MCP: grafanaConnecting},
	"getGrafanaConnection":    {MCP: notBuilt, CLI: notBuilt},
	"updateGrafanaConnection": {MCP: grafanaConnecting, CLI: notBuilt},
	"deleteGrafanaConnection": {MCP: grafanaConnecting, CLI: notBuilt},
	"testGrafanaConnection":   {MCP: grafanaConnecting},
	"listGrafanaFolders":      {MCP: notBuilt},
	"listGrafanaDatasources":  {MCP: notBuilt, CLI: notBuilt},
	"startGrafanaImport":      {MCP: notBuilt},
	"getGrafanaImport":        {MCP: notBuilt},
	"cancelGrafanaImport":     {MCP: notBuilt, CLI: notBuilt},
	"resolveGrafanaUID":       {MCP: notBuilt},
	"fetchGrafanaLink":        {MCP: notBuilt, CLI: notBuilt},

	"listMyGrants":      {MCP: connectedAgents, CLI: connectedAgents},
	"updateMyGrant":     {MCP: connectedAgents, CLI: connectedAgents},
	"disconnectMyGrant": {MCP: connectedAgents, CLI: connectedAgents},
	"listGrants":        {MCP: connectedAgents, CLI: connectedAgents},
	"disconnectGrant":   {MCP: connectedAgents, CLI: connectedAgents},
	"listAgents":        {MCP: connectedAgents, CLI: personOnly},
	"listAgentRuns":     {MCP: connectedAgents, CLI: personOnly},
	"listWorkingRuns":   {MCP: connectedAgents, CLI: personOnly},

	"listInbox":         {CLI: personOnly},
	"markInboxRead":     {MCP: personOnly, CLI: personOnly},
	"setInboxItemState": {CLI: personOnly},

	"requestHealthRepairs": {MCP: notBuilt},

	"previewImport":      {MCP: notBuilt},
	"listImports":        {MCP: notBuilt, CLI: notBuilt},
	"getDashboardImport": {MCP: notBuilt, CLI: notBuilt},

	"getNewsFeed": {MCP: personOnly, CLI: personOnly},

	"getConnectionRequest":     {MCP: consentScreen, CLI: consentScreen},
	"approveConnectionRequest": {MCP: consentScreen, CLI: consentScreen},
	"denyConnectionRequest":    {MCP: consentScreen, CLI: consentScreen},

	"getAgent":                {MCP: notBuilt},
	"saveObserveRequestBuild": {MCP: notBuilt, CLI: notBuilt},
	"continueObserveRequest":  {MCP: notBuilt},
	"getBuildPanelData":       {MCP: personOnly, CLI: personOnly},
	"getBuildVariableValues":  {MCP: personOnly, CLI: personOnly},

	"listOutboundDomains":  {MCP: personOnly, CLI: personOnly},
	"allowOutboundDomain":  {MCP: personOnly, CLI: personOnly},
	"removeOutboundDomain": {MCP: personOnly, CLI: personOnly},

	"previewDraftPanel":       {MCP: personOnly, CLI: personOnly},
	"previewObservationBlock": {MCP: personOnly, CLI: personOnly},

	"listSavedPrompts":  {MCP: notBuilt},
	"createSavedPrompt": {MCP: personOnly, CLI: personOnly},
	"updateSavedPrompt": {MCP: personOnly, CLI: personOnly},
	"deleteSavedPrompt": {MCP: personOnly, CLI: personOnly},

	"getProvisionStatus":    {MCP: notBuilt, CLI: notBuilt},
	"runProvisioning":       {MCP: notBuilt, CLI: notBuilt},
	"applyDashboardFile":    {MCP: notBuilt},
	"listAppliedDashboards": {MCP: notBuilt},

	"listPublicLinks":         {MCP: publicLink, CLI: publicLink},
	"createPublicLink":        {MCP: publicLink, CLI: publicLink},
	"deletePublicLink":        {MCP: publicLink, CLI: publicLink},
	"getPublicDashboard":      {MCP: publicView, CLI: publicView},
	"getPublicPanelData":      {MCP: publicView, CLI: publicView},
	"getPublicVariableValues": {MCP: publicView, CLI: publicView},

	"getPanelData":      {MCP: webRendering, CLI: webRendering},
	"getVariableValues": {MCP: webRendering, CLI: webRendering},

	"listRepairs":    {MCP: notBuilt, CLI: notBuilt},
	"requestRepairs": {MCP: notBuilt, CLI: notBuilt},

	"createDashboardRevision": {MCP: notBuilt},
	"listDashboardRevisions":  {MCP: builtInRuns},
	"getDashboardRevision":    {MCP: builtInRuns},

	"deleteSavedSearch": {MCP: savedSearchCuration},

	"createServiceAccount":     {MCP: accountKeys, CLI: accountKeys},
	"listServiceAccounts":      {MCP: accountKeys, CLI: accountKeys},
	"updateServiceAccountRole": {MCP: accountKeys, CLI: accountKeys},
	"createServiceAccountKey":  {MCP: accountKeys, CLI: accountKeys},
	"listServiceAccountKeys":   {MCP: accountKeys, CLI: accountKeys},
	"revokeServiceAccountKey":  {MCP: accountKeys, CLI: accountKeys},
	"deleteServiceAccount":     {MCP: accountKeys, CLI: accountKeys},

	"starDashboard":   {CLI: personOnly},
	"unstarDashboard": {CLI: personOnly},
	"listStars":       {CLI: personOnly},

	"getHomeDashboard":   {CLI: personOnly},
	"setHomeDashboard":   {CLI: personOnly},
	"clearHomeDashboard": {CLI: personOnly},

	"createPanelSuggestion": {MCP: notBuilt, CLI: notBuilt},
	"getPanelSuggestion":    {MCP: notBuilt, CLI: notBuilt},

	"tailLogs": {MCP: liveWebPage},

	"listUsers":             {MCP: peopleAdmin},
	"updatePersonRole":      {MCP: personOnly, CLI: personOnly},
	"signOutUserEverywhere": {MCP: personOnly, CLI: personOnly},
	"deactivatePerson":      {MCP: endsSignIns, CLI: endsSignIns},
	"reactivatePerson":      {MCP: endsSignIns, CLI: endsSignIns},
	"removePerson":          {MCP: noMCPDeletes, CLI: removesPeople},
	"listInvites":           {MCP: peopleAdmin},
	"createInvite":          {MCP: peopleAdmin},
	"deleteInvite":          {MCP: noMCPDeletes},
	"listTeams":             {MCP: peopleAdmin},
	"createTeam":            {MCP: peopleAdmin},
	"deleteTeam":            {MCP: noMCPDeletes},
	"addTeamMember":         {MCP: peopleAdmin},
	"removeTeamMember":      {MCP: noMCPDeletes},

	"getDatasourceConfig":               {MCP: datasourceAdmin},
	"createDatasource":                  {MCP: datasourceAdmin},
	"updateDatasource":                  {MCP: datasourceAdmin},
	"deleteDatasource":                  {MCP: datasourceAdmin},
	"testDatasource":                    {MCP: datasourceAdmin},
	"listContextSources":                {MCP: contextSourceAdmin},
	"createContextSource":               {MCP: contextSourceAdmin},
	"getContextSource":                  {MCP: contextSourceAdmin},
	"updateContextSource":               {MCP: contextSourceAdmin},
	"deleteContextSource":               {MCP: contextSourceAdmin},
	"discoverContextSourceTools":        {MCP: contextSourceAdmin},
	"listContextSourceTools":            {MCP: contextSourceAdmin},
	"approveReadOnlyContextSourceTools": {MCP: contextSourceAdmin},
	"approveContextSourceTool":          {MCP: contextSourceAdmin},
	"revokeContextSourceTool":           {MCP: contextSourceAdmin},
	"listWebhooks":                      {MCP: webhookAdmin},
	"createWebhook":                     {MCP: webhookAdmin},
	"updateWebhook":                     {MCP: webhookAdmin},
	"deleteWebhook":                     {MCP: webhookAdmin},
	"rotateWebhookSecret":               {MCP: webhookAdmin},
	"testWebhook":                       {MCP: webhookAdmin},
	"skipWebhookEvent":                  {MCP: webhookAdmin},
	"listWebhookDeliveries":             {MCP: webhookAdmin},
}

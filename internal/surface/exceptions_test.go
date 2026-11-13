package surface

type exception struct{ MCP, CLI string }

const (
	savedSearchCuration = "people curate saved searches; an agent saves and lists them"
	builtInRuns         = "an outside agent does its own work over MCP; the built-in agent's runs are read in the web app and the CLI, and their events and cost reach MCP through list_activity"
	consentScreen       = "browser consent screen"
	connectedAgents     = "connected agents are a person's credentials; keys never manage credentials, the CLI signs in with a key, and no MCP tool manages a person's credentials"
	webhookAdmin        = "An Admin decides where workspace activity is sent; an agent that could register a webhook could send it anywhere."
	publicLink          = "a public link is a person's decision: no agent or MCP tool creates, lists or revokes one"
	noMCPDeletes        = "no MCP tool deletes anything: every MCP write is advertised as non-destructive, so people delete in the web app, the HTTP API or the CLI"
	panelActions        = "runs a panel action a person clicks; the agent and MCP clients never add, change or run actions"
	builtInAgentQueue   = "starts or steers a run of the built-in agent; an outside agent does the work itself with its own model and the same tools"
	browserSignIn       = "the browser's sign-in and session flow; keys and MCP clients authenticate with a bearer token"
	personOnly          = "a person's own action in the web app: keys are refused (auth.RequireUser or 403 user_session_required), and the CLI signs in with a key"
	openRecording       = "dashboard opens are recorded for people only; a key that posts one records nothing"
	accountKeys         = "keys never create, list or revoke keys (auth.RequireUser), and no MCP tool manages credentials"
	webRendering        = "the web app's rendering of a saved dashboard or panel; agents run queries with query_range and query_instant, and the CLI with observe logs and observe traces"
	liveWebPage         = "a live event stream for an open web page; MCP is stateless and does not stream"
	publicView          = "an anonymous read through a public link token; there is no principal"
	notBuilt            = "no tool or command yet"
)

var exceptions = map[string]exception{
	"runAction": {MCP: panelActions, CLI: panelActions},

	"deleteAnnotation": {MCP: noMCPDeletes},

	"getAssetImage":   {MCP: personOnly, CLI: personOnly},
	"getAssetGeojson": {MCP: personOnly, CLI: personOnly},

	"authLogin":    {MCP: browserSignIn, CLI: browserSignIn},
	"authCallback": {MCP: browserSignIn, CLI: browserSignIn},
	"authLogout":   {MCP: browserSignIn, CLI: browserSignIn},
	"authSession":  {MCP: browserSignIn, CLI: browserSignIn},
	"getMe":        {MCP: notBuilt},
	"getInstance":  {MCP: notBuilt},

	"createDashboard":        {CLI: notBuilt},
	"listDashboardTags":      {MCP: notBuilt, CLI: notBuilt},
	"deleteDashboard":        {MCP: noMCPDeletes},
	"createDashboardVersion": {CLI: notBuilt},
	"recordDashboardView":    {MCP: openRecording, CLI: openRecording},

	"streamDashboardEvents": {MCP: liveWebPage, CLI: liveWebPage},

	"logVolume": {MCP: notBuilt, CLI: notBuilt},

	"listFindings":           {MCP: notBuilt, CLI: notBuilt},
	"getFinding":             {MCP: notBuilt, CLI: notBuilt},
	"pinFinding":             {MCP: notBuilt, CLI: notBuilt},
	"saveFindingAsDashboard": {MCP: notBuilt, CLI: notBuilt},
	"getFindingPanelData":    {MCP: personOnly, CLI: personOnly},

	"updateObservationBlock": {CLI: notBuilt},
	"removeObservationBlock": {MCP: noMCPDeletes, CLI: notBuilt},
	"getSnapshot":            {MCP: webRendering, CLI: webRendering},
	"observationBlockData":   {MCP: webRendering, CLI: webRendering},
	"listRcaPublicLinks":     {MCP: publicLink, CLI: publicLink},
	"createRcaPublicLink":    {MCP: publicLink, CLI: publicLink},
	"deleteRcaPublicLink":    {MCP: publicLink, CLI: publicLink},
	"viewPublicRca":          {MCP: publicView, CLI: publicView},
	"viewPublicRcaSnapshot":  {MCP: publicView, CLI: publicView},

	"createFolder":         {MCP: notBuilt},
	"getFolder":            {MCP: notBuilt, CLI: notBuilt},
	"renameFolder":         {MCP: notBuilt},
	"deleteFolder":         {MCP: noMCPDeletes},
	"getFolderPermissions": {MCP: notBuilt, CLI: notBuilt},
	"setFolderPermissions": {MCP: notBuilt, CLI: notBuilt},

	"listGrafanaConnections":  {MCP: notBuilt, CLI: notBuilt},
	"createGrafanaConnection": {MCP: notBuilt},
	"getGrafanaConnection":    {MCP: notBuilt, CLI: notBuilt},
	"updateGrafanaConnection": {MCP: notBuilt, CLI: notBuilt},
	"deleteGrafanaConnection": {MCP: notBuilt, CLI: notBuilt},
	"testGrafanaConnection":   {MCP: notBuilt},
	"listGrafanaFolders":      {MCP: notBuilt},
	"listGrafanaDatasources":  {MCP: notBuilt, CLI: notBuilt},
	"startGrafanaImport":      {MCP: notBuilt},
	"getGrafanaImport":        {MCP: notBuilt},
	"cancelGrafanaImport":     {MCP: notBuilt, CLI: notBuilt},
	"resolveGrafanaUID":       {MCP: notBuilt},

	"listMyGrants":      {MCP: connectedAgents, CLI: connectedAgents},
	"updateMyGrant":     {MCP: connectedAgents, CLI: connectedAgents},
	"disconnectMyGrant": {MCP: connectedAgents, CLI: connectedAgents},
	"listGrants":        {MCP: connectedAgents, CLI: connectedAgents},
	"disconnectGrant":   {MCP: connectedAgents, CLI: connectedAgents},
	"listAgents":        {MCP: connectedAgents, CLI: personOnly},
	"listAgentRuns":     {MCP: connectedAgents, CLI: personOnly},

	"listInbox":     {CLI: personOnly},
	"markInboxRead": {MCP: personOnly, CLI: personOnly},

	"requestHealthRepairs": {MCP: builtInAgentQueue},
	"runDashboardHealth":   {MCP: notBuilt},

	"previewImport":      {MCP: notBuilt},
	"listImports":        {MCP: notBuilt, CLI: notBuilt},
	"getDashboardImport": {MCP: notBuilt, CLI: notBuilt},

	"listDatasourceMetrics":     {CLI: notBuilt},
	"listDatasourceLabels":      {CLI: notBuilt},
	"listDatasourceLabelValues": {CLI: notBuilt},

	"getNewsFeed": {MCP: personOnly, CLI: personOnly},

	"getConnectionRequest":     {MCP: consentScreen, CLI: consentScreen},
	"approveConnectionRequest": {MCP: consentScreen, CLI: consentScreen},
	"denyConnectionRequest":    {MCP: consentScreen, CLI: consentScreen},

	"getAgent":                {MCP: notBuilt},
	"listObserveRequests":     {MCP: builtInRuns, CLI: notBuilt},
	"createObserveRequest":    {MCP: builtInAgentQueue},
	"updateObserveRequest":    {MCP: builtInAgentQueue, CLI: notBuilt},
	"archiveObserveRequest":   {MCP: builtInAgentQueue, CLI: notBuilt},
	"getObserveRequestBuild":  {MCP: builtInRuns},
	"saveObserveRequestBuild": {MCP: builtInAgentQueue, CLI: notBuilt},
	"cancelObserveRequest":    {MCP: builtInAgentQueue},
	"retryObserveRequest":     {MCP: builtInAgentQueue},
	"continueObserveRequest":  {MCP: builtInAgentQueue},
	"getBuildPanelData":       {MCP: personOnly, CLI: personOnly},
	"getBuildVariableValues":  {MCP: personOnly, CLI: personOnly},

	"listOutboundDomains":  {MCP: personOnly, CLI: personOnly},
	"allowOutboundDomain":  {MCP: personOnly, CLI: personOnly},
	"removeOutboundDomain": {MCP: personOnly, CLI: personOnly},

	"previewDraftPanel": {MCP: personOnly, CLI: personOnly},

	"listSavedPrompts":  {MCP: builtInAgentQueue},
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

	"getPanelData":        {MCP: webRendering, CLI: webRendering},
	"getVariableValues":   {MCP: webRendering, CLI: webRendering},
	"lintDatasourceQuery": {CLI: notBuilt},

	"listRepairs":    {MCP: notBuilt, CLI: notBuilt},
	"requestRepairs": {MCP: builtInAgentQueue, CLI: notBuilt},

	"createDashboardRevision": {MCP: builtInAgentQueue},
	"listDashboardRevisions":  {MCP: builtInRuns, CLI: notBuilt},
	"getDashboardRevision":    {MCP: builtInRuns, CLI: notBuilt},

	"updateSavedSearch": {MCP: savedSearchCuration},
	"deleteSavedSearch": {MCP: savedSearchCuration},

	"createServiceAccount":     {MCP: accountKeys, CLI: accountKeys},
	"listServiceAccounts":      {MCP: accountKeys, CLI: accountKeys},
	"updateServiceAccountRole": {MCP: accountKeys, CLI: accountKeys},
	"createServiceAccountKey":  {MCP: accountKeys, CLI: accountKeys},
	"listServiceAccountKeys":   {MCP: accountKeys, CLI: accountKeys},
	"revokeServiceAccountKey":  {MCP: accountKeys, CLI: accountKeys},

	"starDashboard":   {CLI: personOnly},
	"unstarDashboard": {CLI: personOnly},
	"listStars":       {CLI: personOnly},

	"createPanelSuggestion": {MCP: builtInAgentQueue, CLI: notBuilt},
	"getPanelSuggestion":    {MCP: builtInAgentQueue, CLI: notBuilt},

	"tailLogs": {MCP: liveWebPage},

	"traceLogs": {CLI: notBuilt},

	"traceStats": {CLI: notBuilt},

	"listUsers":             {MCP: personOnly, CLI: personOnly},
	"updatePersonRole":      {MCP: personOnly, CLI: personOnly},
	"signOutUserEverywhere": {MCP: personOnly, CLI: personOnly},

	"listWebhooks":          {MCP: webhookAdmin},
	"createWebhook":         {MCP: webhookAdmin},
	"updateWebhook":         {MCP: webhookAdmin},
	"deleteWebhook":         {MCP: webhookAdmin},
	"rotateWebhookSecret":   {MCP: webhookAdmin},
	"testWebhook":           {MCP: webhookAdmin},
	"listWebhookDeliveries": {MCP: webhookAdmin},
}

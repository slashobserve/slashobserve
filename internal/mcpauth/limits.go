package mcpauth

import "time"

const (
	accessTTL              = time.Hour
	refreshTTL             = 30 * 24 * time.Hour
	grantMaxAge            = 90 * 24 * time.Hour
	codeTTL                = time.Minute
	requestTTL             = 10 * time.Minute
	refreshReuseLeeway     = 10 * time.Second
	maxPendingRequests     = 10000
	maxUnusedClients       = 1000
	maxLiveGrantsPerPerson = 20
	grantTouchInterval     = 5 * time.Minute
	grantTouchTimeout      = 2 * time.Second
	maxFormBytes           = 64 << 10
	maxRegistrationBytes   = 64 << 10
	maxRedirectURIBytes    = 2048
	maxRedirectURIs        = 10
	maxStateBytes          = 1024
	pruneBatch             = 10000
)

const (
	registerPerMinute       = 30
	authorizePerMinute      = 120
	tokenPerClientPerMinute = 120
	tokenPerMinute          = 1200
	revokePerMinute         = 120
	limitWindow             = time.Minute
)

const (
	accessPrefix  = "soa_"
	refreshPrefix = "sor_"
	codePrefix    = "soc_"
)

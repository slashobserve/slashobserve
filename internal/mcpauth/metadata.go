package mcpauth

import (
	"net/http"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

type serverMetadata struct {
	Issuer                                     string   `json:"issuer"`
	AuthorizationEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                              string   `json:"token_endpoint"`
	RegistrationEndpoint                       string   `json:"registration_endpoint"`
	RevocationEndpoint                         string   `json:"revocation_endpoint"`
	ResponseTypesSupported                     []string `json:"response_types_supported"`
	GrantTypesSupported                        []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported              []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported          []string `json:"token_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported     []string `json:"revocation_endpoint_auth_methods_supported"`
	AuthorizationResponseIssParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
}

func (s *Server) protectedResourceMetadata() http.HandlerFunc {
	handler := mcpserver.NewProtectedResourceMetadataHandler(mcpserver.ProtectedResourceMetadataConfig{
		Resource:               s.resource,
		AuthorizationServers:   []string{s.issuer},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "SlashObserve",
	})
	return handler.ServeHTTP
}

func (s *Server) authorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, MCP-Protocol-Version")
	h.Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet, http.MethodHead:
		writeJSON(w, http.StatusOK, serverMetadata{
			Issuer:                                     s.issuer,
			AuthorizationEndpoint:                      s.issuer + "/oauth/authorize",
			TokenEndpoint:                              s.issuer + "/oauth/token",
			RegistrationEndpoint:                       s.issuer + "/oauth/register",
			RevocationEndpoint:                         s.issuer + "/oauth/revoke",
			ResponseTypesSupported:                     []string{"code"},
			GrantTypesSupported:                        []string{"authorization_code", "refresh_token"},
			CodeChallengeMethodsSupported:              []string{"S256"},
			TokenEndpointAuthMethodsSupported:          []string{"none"},
			RevocationEndpointAuthMethodsSupported:     []string{"none"},
			AuthorizationResponseIssParameterSupported: true,
		})
	default:
		h.Set("Allow", "GET, HEAD, OPTIONS")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

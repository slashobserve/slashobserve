package server

import (
	"fmt"
	"net/netip"

	"github.com/gin-gonic/gin"
)

func trustProxies(engine *gin.Engine, proxies []netip.Prefix) error {
	ranges := make([]string, len(proxies))
	for i, p := range proxies {
		ranges[i] = p.String()
	}
	engine.ForwardedByClientIP = len(ranges) > 0
	engine.RemoteIPHeaders = []string{"X-Forwarded-For"}
	if err := engine.SetTrustedProxies(ranges); err != nil {
		return fmt.Errorf("trust proxies: %w", err)
	}
	return nil
}

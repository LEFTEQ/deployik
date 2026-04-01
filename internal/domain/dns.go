package domain

import (
	"fmt"
	"net"
)

// VerifyDNS checks if a domain's A record points to the expected IP.
func VerifyDNS(domainName, expectedIP string) (bool, error) {
	ips, err := net.LookupHost(domainName)
	if err != nil {
		return false, fmt.Errorf("DNS lookup for %s: %w", domainName, err)
	}

	for _, ip := range ips {
		if ip == expectedIP {
			return true, nil
		}
	}

	return false, nil
}

package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type destinationPolicy struct {
	allow []netip.Prefix
	block []netip.Prefix
}

func parseDestinationPolicy(allow, block []string) (*destinationPolicy, error) {
	if len(allow) == 0 && len(block) == 0 {
		return nil, nil
	}
	policy := &destinationPolicy{}
	for _, item := range allow {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, fmt.Errorf("invalid destination_allow CIDR %q: %w", item, err)
		}
		policy.allow = append(policy.allow, prefix.Masked())
	}
	for _, item := range block {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, fmt.Errorf("invalid destination_block CIDR %q: %w", item, err)
		}
		policy.block = append(policy.block, prefix.Masked())
	}
	return policy, nil
}

func (p *destinationPolicy) allows(addr netip.Addr) bool {
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	for _, prefix := range p.block {
		if prefix.Contains(addr) {
			return false
		}
	}
	if len(p.allow) == 0 {
		return true
	}
	for _, prefix := range p.allow {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

type destinationPolicyDialer struct {
	policy *destinationPolicy
}

func (d destinationPolicyDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d destinationPolicyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address %q: %w", address, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("invalid destination port %q", port)
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve destination %q: %w", host, err)
	}
	var rejected int
	var lastErr error
	dialer := new(net.Dialer)
	for _, ip := range ips {
		if !d.policy.allows(ip) {
			rejected++
			continue
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if rejected > 0 && rejected == len(ips) {
		return nil, errors.New("destination address rejected by CIDR policy")
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("destination has no IP addresses allowed by CIDR policy")
}

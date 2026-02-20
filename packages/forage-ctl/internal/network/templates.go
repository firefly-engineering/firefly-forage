package network

import (
	"text/template"
)

// nftablesData holds data for the nftables template.
type nftablesData struct {
	IPv4Addrs string // comma-separated list of allowed IPv4 addresses
	IPv6Addrs string // comma-separated list of allowed IPv6 addresses
}

// dnsmasqData holds data for the dnsmasq template.
type dnsmasqData struct {
	ServerLines string // newline-separated server= directives
}

var nftablesTmpl = template.Must(template.New("nftables").Parse(`#!/usr/sbin/nft -f

# Flush existing rules
flush ruleset

table inet filter {
  # Set of allowed IPv4 addresses
  set allowed_ipv4 {
    type ipv4_addr
    flags interval
    elements = { {{.IPv4Addrs}} }
  }

  # Set of allowed IPv6 addresses
  set allowed_ipv6 {
    type ipv6_addr
    flags interval
    elements = { {{.IPv6Addrs}} }
  }

  chain input {
    type filter hook input priority 0; policy accept;
  }

  chain forward {
    type filter hook forward priority 0; policy accept;
  }

  chain output {
    type filter hook output priority 0; policy drop;

    # Allow loopback
    oif "lo" accept

    # Allow established/related connections
    ct state established,related accept

    # Allow ICMP for diagnostics
    ip protocol icmp accept
    ip6 nexthdr icmpv6 accept

    # Allow DNS to local resolver only (localhost)
    tcp dport 53 ip daddr 127.0.0.1 accept
    udp dport 53 ip daddr 127.0.0.1 accept

    # Allow connections to allowed IPv4 addresses
    ip daddr @allowed_ipv4 accept

    # Allow connections to allowed IPv6 addresses
    ip6 daddr @allowed_ipv6 accept

    # Log and reject everything else
    log prefix "forage-blocked: " level info
    reject with icmp type admin-prohibited
  }
}
`))

var dnsmasqTmpl = template.Must(template.New("dnsmasq").Parse(`# Forage DNS filtering configuration
# Only resolve allowed hosts, block everything else

# Don't read /etc/resolv.conf
no-resolv

# Don't read /etc/hosts
no-hosts

# Listen only on localhost
listen-address=127.0.0.1
bind-interfaces

# Port
port=53

# Upstream DNS servers for allowed domains
{{.ServerLines}}
# Block all other DNS queries by returning NXDOMAIN
address=/#/

# Cache settings
cache-size=1000

# Log queries (optional, useful for debugging)
# log-queries

# Don't forward plain names (without dots)
domain-needed

# Never forward addresses in non-routed address spaces
bogus-priv
`))

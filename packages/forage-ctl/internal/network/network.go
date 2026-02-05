// Package network provides network isolation configuration for sandboxes
package network

import (
	"fmt"
	"net"
	"strings"
)

// Mode represents the network isolation mode
type Mode string

const (
	ModeFull       Mode = "full"
	ModeRestricted Mode = "restricted"
	ModeNone       Mode = "none"
)

// Config holds network configuration for a sandbox
type Config struct {
	Mode         Mode
	AllowedHosts []string
	NetworkSlot  int
}

// ResolvedHost contains a hostname and its resolved IPs
type ResolvedHost struct {
	Hostname string
	IPs      []string
}

// ResolveHosts resolves hostnames to IP addresses
func ResolveHosts(hosts []string) ([]ResolvedHost, error) {
	var resolved []ResolvedHost

	for _, host := range hosts {
		ips, err := net.LookupIP(host)
		if err != nil {
			// If we can't resolve, include the hostname anyway
			// nftables can do DNS resolution at rule load time
			resolved = append(resolved, ResolvedHost{
				Hostname: host,
				IPs:      []string{},
			})
			continue
		}

		var ipStrings []string
		for _, ip := range ips {
			// Include both IPv4 and IPv6
			ipStrings = append(ipStrings, ip.String())
		}

		resolved = append(resolved, ResolvedHost{
			Hostname: host,
			IPs:      ipStrings,
		})
	}

	return resolved, nil
}

// GenerateNftablesRules generates nftables rules for restricted mode
func GenerateNftablesRules(cfg *Config) string {
	if cfg.Mode != ModeRestricted || len(cfg.AllowedHosts) == 0 {
		return ""
	}

	// Resolve hosts to IPs
	resolved, _ := ResolveHosts(cfg.AllowedHosts)

	// Build IP sets
	var ipv4Addrs []string
	var ipv6Addrs []string

	for _, h := range resolved {
		for _, ip := range h.IPs {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if parsed.To4() != nil {
				ipv4Addrs = append(ipv4Addrs, ip)
			} else {
				ipv6Addrs = append(ipv6Addrs, ip)
			}
		}
	}

	// Also allow the hosts by hostname (nftables will resolve)
	var hostnames []string
	hostnames = append(hostnames, cfg.AllowedHosts...)

	// Gateway IP for the container
	gatewayIP := fmt.Sprintf("10.100.%d.1", cfg.NetworkSlot)

	// Build nftables ruleset
	var rules strings.Builder

	rules.WriteString(`#!/usr/sbin/nft -f

# Flush existing rules
flush ruleset

table inet filter {
  # Set of allowed IPv4 addresses
  set allowed_ipv4 {
    type ipv4_addr
    flags interval
    elements = { `)

	// Add gateway and resolved IPs
	ipv4Set := []string{gatewayIP}
	ipv4Set = append(ipv4Set, ipv4Addrs...)
	// Add common infrastructure IPs (DNS, etc.)
	ipv4Set = append(ipv4Set, "127.0.0.1")

	rules.WriteString(strings.Join(ipv4Set, ", "))
	rules.WriteString(` }
  }

  # Set of allowed IPv6 addresses
  set allowed_ipv6 {
    type ipv6_addr
    flags interval
    elements = { ::1`)

	if len(ipv6Addrs) > 0 {
		rules.WriteString(", ")
		rules.WriteString(strings.Join(ipv6Addrs, ", "))
	}

	rules.WriteString(` }
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
`)

	return rules.String()
}

// GenerateDnsmasqConfig generates dnsmasq configuration for DNS filtering
func GenerateDnsmasqConfig(allowedHosts []string) string {
	var config strings.Builder

	config.WriteString(`# Forage DNS filtering configuration
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
`)

	// For each allowed host, forward to public DNS
	for _, host := range allowedHosts {
		// Handle wildcards (e.g., *.anthropic.com)
		if strings.HasPrefix(host, "*.") {
			domain := strings.TrimPrefix(host, "*.")
			config.WriteString(fmt.Sprintf("server=/%s/1.1.1.1\n", domain))
			config.WriteString(fmt.Sprintf("server=/%s/8.8.8.8\n", domain))
		} else {
			// For specific hosts, we need to extract the domain
			config.WriteString(fmt.Sprintf("server=/%s/1.1.1.1\n", host))
			config.WriteString(fmt.Sprintf("server=/%s/8.8.8.8\n", host))
		}
	}

	config.WriteString(`
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
`)

	return config.String()
}

// GenerateNixNetworkConfig generates NixOS configuration for network isolation
func GenerateNixNetworkConfig(cfg *Config) string {
	switch cfg.Mode {
	case ModeNone:
		return generateNoneConfig()
	case ModeRestricted:
		return generateRestrictedConfig(cfg)
	default: // ModeFull
		return generateFullConfig(cfg.NetworkSlot)
	}
}

func generateNoneConfig() string {
	return `# No network access
      networking.nameservers = [];
      networking.defaultGateway = null;

      # Disable all network interfaces except loopback
      networking.useDHCP = false;

      # Block all outgoing traffic
      networking.firewall = {
        enable = true;
        allowedTCPPorts = [ 22 ];  # Keep SSH for management
        extraCommands = ''
          # Drop all outgoing traffic except to localhost
          iptables -A OUTPUT -o lo -j ACCEPT
          iptables -A OUTPUT -j DROP
        '';
      };`
}

func generateRestrictedConfig(cfg *Config) string {
	if len(cfg.AllowedHosts) == 0 {
		return generateNoneConfig()
	}

	// Build the list of allowed hosts for nftables
	var allowedIPv4 []string
	var allowedIPv6 []string

	// Always allow gateway
	gatewayIP := fmt.Sprintf("10.100.%d.1", cfg.NetworkSlot)
	allowedIPv4 = append(allowedIPv4, gatewayIP, "127.0.0.1")
	allowedIPv6 = append(allowedIPv6, "::1")

	// Resolve hosts
	resolved, _ := ResolveHosts(cfg.AllowedHosts)
	for _, h := range resolved {
		for _, ip := range h.IPs {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if parsed.To4() != nil {
				allowedIPv4 = append(allowedIPv4, ip)
			} else {
				allowedIPv6 = append(allowedIPv6, ip)
			}
		}
	}

	// Build dnsmasq server lines
	var dnsServers []string
	for _, host := range cfg.AllowedHosts {
		if strings.HasPrefix(host, "*.") {
			domain := strings.TrimPrefix(host, "*.")
			dnsServers = append(dnsServers, fmt.Sprintf("server=/%s/1.1.1.1", domain))
		} else {
			dnsServers = append(dnsServers, fmt.Sprintf("server=/%s/1.1.1.1", host))
		}
	}

	return fmt.Sprintf(`# Restricted network - only allowed hosts
      networking.defaultGateway = "%s";
      networking.nameservers = [ "127.0.0.1" ];  # Use local DNS filter

      # DNS filtering with dnsmasq
      services.dnsmasq = {
        enable = true;
        settings = {
          # Don't use system resolv.conf
          no-resolv = true;

          # Listen only on localhost
          listen-address = "127.0.0.1";
          bind-interfaces = true;

          # Forward allowed domains to public DNS
          server = [
            %s
          ];

          # Block all other queries
          address = "/#/";

          # Cache settings
          cache-size = 1000;

          # Security
          domain-needed = true;
          bogus-priv = true;
        };
      };

      # nftables for egress filtering
      networking.nftables = {
        enable = true;
        ruleset = ''
          table inet filter {
            set allowed_ipv4 {
              type ipv4_addr
              flags interval
              elements = { %s }
            }

            set allowed_ipv6 {
              type ipv6_addr
              flags interval
              elements = { %s }
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

              # Allow established/related
              ct state established,related accept

              # Allow ICMP
              ip protocol icmp accept
              ip6 nexthdr icmpv6 accept

              # Allow DNS to local resolver
              tcp dport 53 ip daddr 127.0.0.1 accept
              udp dport 53 ip daddr 127.0.0.1 accept

              # Allow connections to allowed hosts
              ip daddr @allowed_ipv4 accept
              ip6 daddr @allowed_ipv6 accept

              # Reject everything else
              reject with icmp type admin-prohibited
            }
          }
        '';
      };

      # Disable iptables (using nftables instead)
      networking.firewall.enable = false;`,
		gatewayIP,
		formatNixList(dnsServers),
		strings.Join(allowedIPv4, ", "),
		strings.Join(allowedIPv6, ", "),
	)
}

func generateFullConfig(slot int) string {
	return fmt.Sprintf(`# Full network access
      networking.defaultGateway = "10.100.%d.1";
      networking.nameservers = [ "1.1.1.1" "8.8.8.8" ];`, slot)
}

func formatNixList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, "\n            ")
}

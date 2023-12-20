package basecrawler

import (
	"log/slog"
	"net"
)

// Special-use IPv4 addresses
// (taken from https://en.wikipedia.org/wiki/Internet_Protocol_version_4#Special-use_addresses)
var specialUseRanges = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"233.252.0.0/24",
	"240.0.0.0/4",
	"255.255.255.255/32",
}

// IsBlacklisted returns whether the crawler considers a bot blacklisted and therefore won't crawl it.
//
// The bot should be provided as ip:port (so that [net.SplitHostPort]).
// If the given bot can't be parsed it will be considered as blacklisted (to make sure the crawler won't crawl any bogon IPs).
//
// By default the blacklist contains all [special-use IP addresses].
// This can be changed by providing the [WithIncludeSpecialUseIPs] option.
// Additional IP ranges can be blacklisted by providing the [WithCustomBlacklist] option.
//
// [special-use IP addresses]: https://en.wikipedia.org/wiki/Internet_Protocol_version_4#Special-use_addresses
func (bc *BaseCrawler) IsBlacklisted(ipPort string) bool {
	ip, _, err := net.SplitHostPort(ipPort)
	if err != nil {
		bc.logger.Warn("Could not split IP/port, considering it as blacklisted",
			slog.String("ip", ipPort),
		)
		return true
	}

	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		bc.logger.Debug("Parsed an IP to the nil IP, considering it as blacklisted",
			slog.String("ip", ip),
		)
		return true
	}

	for _, ipRange := range bc.blacklistedIPRanges {
		if ipRange.Contains(ipAddr) {
			bc.logger.Debug("IP is blacklisted",
				slog.String("ip", ip),
			)
			return true
		}
	}

	return false
}

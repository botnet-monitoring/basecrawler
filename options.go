package basecrawler

import (
	"log/slog"

	bmsclient "github.com/botnet-monitoring/grpc-client"
)

type config struct {
	customBlacklist      []string
	includeSpecialUseIPs bool

	logger *slog.Logger

	discoveryInterval       uint32
	trackingInterval        uint32
	discoveryRemoveInterval uint32
	trackingRemoveInterval  uint32

	discoveryWorkerCount uint32
	trackingWorkerCount  uint32
	findPeerWorkerCount  uint32

	bmsConfig bmsConfig

	additionalConfig map[string]any
}

type bmsConfig struct {
	server     string
	monitorID  string
	authToken  string
	botnetID   string
	campaignID string
	publicIP   string

	bmsOptions []bmsclient.SessionOption
}

// Functions which implement CrawlerOption can be passed to [NewCrawler] as additional options.
type CrawlerOption func(*config)

// Functions which implement CrawlerOption can be passed to [WithBMS] as additional options.
type BMSOption func(*bmsConfig)

// WithCustomBlacklist can be used to add IP address ranges to the crawler's blacklist (see [BaseCrawler.IsBlacklisted]).
//
// The given strings have to be in CIDR notation.
// You might want to exclude your own crawlers from the crawling, so e.g. if you have two crawlers running on 198.51.100.1 and 198.51.100.2, you probably want to pass a string slice with 198.51.100.1/32 and 198.51.100.2/32.
func WithCustomBlacklist(blacklist []string) CrawlerOption {
	return func(c *config) {
		c.customBlacklist = blacklist
	}
}

// WithIncludeSpecialUseIPs can be used to include special-use IP addresses into the crawling (see [BaseCrawler.IsBlacklisted]).
//
// You probably want to leave this setting on its default (so that the blacklist contains the special-use IP addresses), however e.g. for local testing you might want to include them.
func WithIncludeSpecialUseIPs(includeSpecialUseIPs bool) CrawlerOption {
	return func(c *config) {
		c.includeSpecialUseIPs = includeSpecialUseIPs
	}
}

// WithLogger can be used to pass a custom logger to the basecrawler.
// By default, the basecrawler doesn't log anything, so if you want to have any logs, you have to pass this option.
//
// If you also use [CreateOptionsFromEnv], it might create its own logger (depending on the value of LOG_LEVEL) which might overwrite another provided logger (options are applied in the order they are passed).
// If you want to pass the created logger to your crawler implementation, use [BaseCrawler.Logger].
func WithLogger(logger *slog.Logger) CrawlerOption {
	return func(c *config) {
		c.logger = logger
	}
}

// WithCustomCrawlIntervals can be used to change how often the crawler will crawl potential bots and after how long of being unresponsive it will drop them.
//
// All intervals are considered to be seconds.
// If you only want to change on of the intervals, pass zero for the other parameters.
// Defaults are 300s for discovery interval and tracking interval, and 900s for discovery remove interval and tracking remove interval.
func WithCustomCrawlIntervals(discoveryInterval uint32, trackingInterval uint32, discoveryRemoveInterval uint32, trackingRemoveInterval uint32) CrawlerOption {
	return func(c *config) {
		c.discoveryInterval = discoveryInterval
		c.trackingInterval = trackingInterval
		c.discoveryRemoveInterval = discoveryRemoveInterval
		c.trackingRemoveInterval = trackingRemoveInterval
	}
}

// WithCustomWorkerCounts can be used to change the amount of workers the various loops will spawn.
//
// If you only want to change the amount of one of the loops, pass zero for the other parameters.
// By default the discovery loop and tracking loop will spawn 10000 worker Go routines and the find-peer loop will spawn 100 worker Go routines (if it's used).
func WithCustomWorkerCounts(discoveryWorkerCount uint32, trackingWorkerCount uint32, findPeerWorkerCount uint32) CrawlerOption {
	return func(c *config) {
		c.discoveryWorkerCount = discoveryWorkerCount
		c.trackingWorkerCount = trackingWorkerCount
		c.findPeerWorkerCount = findPeerWorkerCount
	}
}

// WithAdditionalCrawlerConfig can be used to provide custom crawler configuration to the crawler implementation (e.g. SendPeerRequest or ReadReply).
//
// The given config map will be passed as-is to all functions contained in [TCPBotnetCrawler], [UDPBotnetCrawler] and [PeerFinder].
func WithAdditionalCrawlerConfig(crawlerConfig map[string]any) CrawlerOption {
	return func(c *config) {
		c.additionalConfig = crawlerConfig
	}
}

// WithBMS can be used to configure the crawler to send the crawling results to a BMS server.
//
// The server has to be passed as ip:port (e.g. localhost:8083), the authToken as base64-encoded string (which has contain exactly 32 bytes).
// The given monitor ID and botnet ID have to exist on the BMS server, otherwise trying to start the crawler will return an error.
//
// You can pass optional BMS config as last parameter via [BMSOption].
//
// Please note that the option cannot check validity of the used values, so creating the BMS client or server might fail when trying to start the crawler.
func WithBMS(server string, monitor string, authToken string, botnet string, options ...BMSOption) CrawlerOption {
	return func(c *config) {
		bmsConfig := &bmsConfig{
			server:    server,
			monitorID: monitor,
			authToken: authToken,
			botnetID:  botnet,
		}

		for _, option := range options {
			option(bmsConfig)
		}

		c.bmsConfig = *bmsConfig
	}
}

// WithBMSCampaign can be used to set a campaign when sending results to BMS.
// Has to be passed to [WithBMS], as it only can be used in combination with a BMS connection.
func WithBMSCampaign(campaign string) BMSOption {
	return func(c *bmsConfig) {
		c.campaignID = campaign
	}
}

// WithBMSCampaign can be used to set the public IP address of a crawler which will be written to the BMS database.
// Has to be passed to [WithBMS], as it only can be used in combination with a BMS connection.
func WithBMSPublicIP(ip string) BMSOption {
	return func(c *bmsConfig) {
		c.publicIP = ip
	}
}

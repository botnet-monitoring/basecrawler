package basecrawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime/debug"
	"strings"
	"time"

	bmsclient "github.com/botnet-monitoring/grpc-client"
)

type bot struct {
	IpPort      string
	LastCrawled time.Time
	LastSeen    time.Time
}

type contextKey int

const (
	// The context key that is used to store as which loop something is executed in (e.g. discovery_loop)
	// Mostly useful for logging
	ContextKeyLoop contextKey = iota

	// The context key that is used to store as which worker ID is executing something
	// Mostly useful for logging
	ContextKeyWorkerID contextKey = iota
)

// The BaseCrawler struct represents a basecrawler instance.
//
// To create a new instance, use [NewCrawler].
type BaseCrawler struct {
	tcpCrawlerImplementation TCPBotnetCrawler
	udpCrawlerImplementation UDPBotnetCrawler

	bmsClient  *bmsclient.Client
	bmsSession *bmsclient.Session

	bootstrapPeers map[string]bool

	blacklistedIPRanges []net.IPNet

	useTCP          bool
	useFindPeerLoop bool

	withBMS       bool
	activeSession bool

	config // Embedding this mostly so that bc.config.logger gets a little bit shorter (all other fields are accessed via bc.config instead of the corresponding shorthand)
}

// NewCrawler creates a new crawler based on the basecrawler which embeds the given specific implementation.
//
// The first parameter is the actual implementation of the botnet protocol.
// The passed struct has to either implement the [TCPBotnetCrawler] or the [UDPBotnetCrawler] interface.
//
// You can make sure your struct implements e.g. the UDPBotnetCrawler interface at compile time by putting the following in your code:
//
//	// Make sure that someImplementation is implementing the UDPBotnetCrawler interface (at compile time)
//	var _ basecrawler.UDPBotnetCrawler = &someImplementation{}
//
// The entries of the bootstrap peerlist have to be in format parsable by [net.SplitHostPort] (oftentimes it's simply ip:port).
//
// You can pass optional config to the crawler as all further parameters.
// All options have to implement the [CrawlerOption] type (see [CrawlerOption] for available options).
// If you want to read optional crawler configuration from environment variables (in a common way), you can use [CreateOptionsFromEnv] (see the [configuration section] in the readme for possible environment variables).
//
// Although calling this method will already start a BMS session (if configured with BMS), it will not start the actual crawling.
// To start it, call [BaseCrawler.Start].
// The main reason for this layout is that at some point we want to introduce crawler instrumentation, i.e. that you have a piece of software that is able to crawl multiple botnets (and therefore contains multiple crawler instances) which is controlled by a central management server.
//
// [configuration section]: https://github.com/botnet-monitoring/basecrawler#configuration
func NewCrawler(botnetImplementation any, bootstrapPeers []string, options ...CrawlerOption) (*BaseCrawler, error) {
	bc := &BaseCrawler{
		bootstrapPeers: make(map[string]bool),
	}

	for _, peer := range bootstrapPeers {
		// Trim potential "" which is in theory valid CSV (and might be introduced by exporting from a database)
		peer = strings.Trim(peer, "\"")

		host, port, err := net.SplitHostPort(peer)
		if err != nil {
			panic(err)
		}

		// Resolve the given IP once to make sure we don't have any hostnames (IsBlacklisted wouldn't like it)
		resolvedHost, err := net.ResolveIPAddr("ip", host)
		if err != nil {
			panic(err)
		}

		resolvedPeer := net.JoinHostPort(resolvedHost.String(), port)
		bc.bootstrapPeers[resolvedPeer] = true
	}

	config := config{}
	for _, option := range options {
		option(&config)
	}
	bc.config = config

	// If initialized without a custom logger, we need to set an no-op logger (because a nil logger panics)
	// See https://github.com/golang/go/issues/62005
	if bc.logger == nil {
		bc.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	tcpCrawler, isTCP := botnetImplementation.(TCPBotnetCrawler)
	udpCrawler, isUDP := botnetImplementation.(UDPBotnetCrawler)
	if !isTCP && !isUDP {
		return nil, errors.New("parameter botnetCrawler has to either implement TCPBotnetCrawler or UDPBotnetCrawler")
	}

	if isTCP {
		bc.logger.Debug("Using TCP")
		bc.tcpCrawlerImplementation = tcpCrawler
		bc.useTCP = true
	} else {
		bc.logger.Debug("Using UDP")
		bc.udpCrawlerImplementation = udpCrawler
	}

	_, isPeerFinder := botnetImplementation.(PeerFinder)
	bc.useFindPeerLoop = isPeerFinder

	// Set defaults where necessary
	if bc.config.discoveryInterval == 0 {
		bc.config.discoveryInterval = 300
	}
	if bc.config.trackingInterval == 0 {
		bc.config.trackingInterval = 300
	}
	if bc.config.discoveryRemoveInterval == 0 {
		bc.config.discoveryRemoveInterval = 900
	}
	if bc.config.trackingRemoveInterval == 0 {
		bc.config.trackingRemoveInterval = 900
	}
	if bc.config.discoveryWorkerCount == 0 {
		bc.config.discoveryWorkerCount = 10000
	}
	if bc.config.trackingWorkerCount == 0 {
		bc.config.trackingWorkerCount = 10000
	}
	if bc.config.findPeerWorkerCount == 0 {
		bc.config.findPeerWorkerCount = 100
	}

	for _, ipRangeStr := range bc.config.customBlacklist {
		_, ipRange, err := net.ParseCIDR(ipRangeStr)
		if err != nil {
			return nil, err
		}
		bc.blacklistedIPRanges = append(bc.blacklistedIPRanges, *ipRange)
	}

	if !bc.config.includeSpecialUseIPs {
		for _, ipRangeStr := range specialUseRanges {
			_, ipRange, err := net.ParseCIDR(ipRangeStr)
			if err != nil {
				return nil, err
			}
			bc.blacklistedIPRanges = append(bc.blacklistedIPRanges, *ipRange)
		}
	}

	if bc.config.bmsConfig.server != "" || bc.config.bmsConfig.monitorID != "" || bc.config.bmsConfig.authToken != "" || bc.config.bmsConfig.botnetID != "" {
		if bc.config.bmsConfig.server == "" {
			return nil, errors.New("some BMS options provided, but not BMS server")
		}
		if bc.config.bmsConfig.monitorID == "" {
			return nil, errors.New("some BMS options provided, but not monitor id")
		}
		if bc.config.bmsConfig.authToken == "" {
			return nil, errors.New("some BMS options provided, but not auth token")
		}
		if bc.config.bmsConfig.botnetID == "" {
			return nil, errors.New("some BMS options provided, but not botnet id")
		}

		bmsOptions, err := configToBMSOptions(config)
		if err != nil {
			return nil, err
		}
		bc.config.bmsConfig.bmsOptions = bmsOptions

		// Create BMS client
		bmsClient, err := bmsclient.NewClient(
			bc.config.bmsConfig.server,
			bc.config.bmsConfig.monitorID,
			bc.config.bmsConfig.authToken,
		)
		if err != nil {
			bc.logger.Error("Could not create the BMS client",
				slog.String("error", err.Error()),
			)
			return nil, err
		}
		bc.bmsClient = bmsClient

		// Create BMS session
		bmsSession, err := bc.bmsClient.NewSession(bc.config.bmsConfig.botnetID, bc.config.bmsConfig.bmsOptions...)
		if err != nil {
			bc.logger.Warn("Could not establish the initial BMS session",
				slog.String("server", bc.config.bmsConfig.server),
				slog.String("error", err.Error()),
			)
			bc.activeSession = false
		} else {
			bc.bmsSession = bmsSession
			bc.activeSession = true
			bc.logger.Info("Initial BMS session established",
				slog.String("server", bc.config.bmsConfig.server),
			)
		}

		bc.withBMS = true
	} else {
		bc.withBMS = false
		bc.logger.Info("Starting crawler without BMS connection")
	}

	bc.logger.Debug("Final crawler configuration",
		slog.Any("config", bc.config),
	)

	return bc, nil
}

func configToBMSOptions(c config) ([]bmsclient.SessionOption, error) {
	var options []bmsclient.SessionOption

	if c.bmsConfig.campaignID != "" {
		options = append(options, bmsclient.WithCampaign(c.bmsConfig.campaignID))
	}
	if c.bmsConfig.publicIP != "" {
		ip := net.ParseIP(c.bmsConfig.publicIP)
		if ip == nil {
			return nil, fmt.Errorf("ip could not be parsed: %v", c.bmsConfig.publicIP)
		}
		options = append(options, bmsclient.WithIP(ip))
	}

	options = append(options, bmsclient.WithFixedFrequency(c.trackingInterval))

	type additionalConfigStruct = struct {
		GoVersion     string `json:"go_version,omitempty"`
		MainPath      string `json:"build_path,omitempty"`
		MainVersion   string `json:"build_version,omitempty"`
		GitCommit     string `json:"git_commit,omitempty"`
		GitCommitTime string `json:"git_commit_time,omitempty"`
		GitModified   string `json:"git_modified,omitempty"`

		CrawlerConfig map[string]any `json:"crawler_config,omitempty"`
	}
	var additionalConfig additionalConfigStruct

	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		additionalConfig = additionalConfigStruct{
			GoVersion:   buildInfo.GoVersion,
			MainPath:    buildInfo.Main.Path,
			MainVersion: buildInfo.Main.Version,
		}

		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				additionalConfig.GitCommit = setting.Value
			}
			if setting.Key == "vcs.time" {
				additionalConfig.GitCommitTime = setting.Value
			}
			if setting.Key == "vcs.modified" {
				additionalConfig.GitModified = setting.Value
			}
		}
	}

	if len(c.additionalConfig) != 0 {
		additionalConfig.CrawlerConfig = c.additionalConfig
	}

	// If at least one of the two additional config contents from above are there, add it as option
	if ok || len(c.additionalConfig) != 0 {
		options = append(options, bmsclient.WithAdditionalConfig(additionalConfig))
	}

	return options, nil
}

// Start starts the crawler instance.
// Depending on the crawler configuration, it will spawn several Go routines:
//   - A Go routine that manages the discovery loop which itself starts more worker Go routines.
//   - A Go routine that manages the tracking loop which itself starts more worker Go routines.
//   - A Go routine that manages the find-peer loop which itself starts more worker Go routines (if the crawler implementation implements the [PeerFinder] interface).
//   - A Go routine that sends crawled bot replies, edges and failed tries to BMS (if configured to use BMS).
//   - A Go routine that tries to reconnect to BMS if the connection broke (if configured to use BMS).
//
// You can stop these Go routines by passing a cancelable context (like in the CancelContext example).
//
// This method is meant for instrumentation of multiple crawler instances (though we never got around to use it).
// The basic idea is to have multiple crawler instances that can be exchanged (stop one crawler, start another one) on the fly by a central management instance.
func (bc *BaseCrawler) Start(ctx context.Context) error {
	if bc.withBMS {
		bmsFlushTicker := time.NewTicker(10 * time.Second)
		go func() {
		outerLoop:
			for {
				select {
				case <-bmsFlushTicker.C:
					if bc.activeSession {
						bc.logger.Info("Flushing",
							slog.Int("bot_replies", bc.bmsSession.GetNumberOfUnsentBotReplies()),
							slog.Int("edges", bc.bmsSession.GetNumberOfUnsentEdges()),
							slog.Int("failed_tries", bc.bmsSession.GetNumberOfUnsentFailedTries()),
						)
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						err := bc.bmsSession.FlushWithContext(ctx)
						if err != nil {
							bc.logger.Warn("Error while flushing",
								slog.String("error", err.Error()),
							)
							bc.activeSession = false
						}
						cancel()
					} else {
						bc.logger.Info("Would flush, but no active session")
					}

				case <-ctx.Done():
					break outerLoop
				}
			}
		}()

		bmsSessionTicker := time.NewTicker(1 * time.Minute)
		go func() {
		outerLoop:
			for {
				select {
				case <-bmsSessionTicker.C:
					if !bc.activeSession {
						bmsSession, err := bc.bmsClient.NewSession(bc.config.bmsConfig.botnetID, bc.config.bmsConfig.bmsOptions...)
						if err != nil {
							bc.logger.Warn("Failed to reestablish session",
								slog.String("server", bc.config.bmsConfig.server),
								slog.String("error", err.Error()),
							)
						} else {
							bc.logger.Info("Reestablished session",
								slog.String("server", bc.config.bmsConfig.server),
							)
							bc.bmsSession = bmsSession
							bc.activeSession = true
						}
					}

				case <-ctx.Done():
					break outerLoop
				}
			}
		}()
	}

	replyChan := make(chan string, 100000)
	trackingLoopChan := make(chan *bot, 100000)
	discoveryLoopChan := make(chan *bot, 1000000)

	if bc.useFindPeerLoop {
		bc.logger.Info("Using findPeerLoop")
		go bc.findPeerLoop(ctx, replyChan)
	}
	go bc.discoveryLoop(ctx, trackingLoopChan, replyChan, discoveryLoopChan)
	go bc.trackingLoop(ctx, trackingLoopChan, replyChan, discoveryLoopChan)

	bc.logger.Info("Crawler successfully started")

	return nil
}

// Stop currently just ends the internal BMS session with the provided disconnect reason (if there's an active BMS session).
// If you want to stop the crawl loops, pass a cancelable context to [BaseCrawler.Start] and cancel it.
//
// This method is meant for instrumentation of multiple crawler instances (though we never got around to use it).
// Please also note that the crawler very likely won't start again once stopped (which we would need to fix before being able to do proper instrumentation).
func (bc *BaseCrawler) Stop(ctx context.Context, reason bmsclient.DisconnectReason) error {
	if bc.withBMS && bc.activeSession {
		err := bc.bmsSession.EndWithContextAndReason(ctx, reason)
		if err != nil {
			return err
		}
	}

	return nil
}

// Logger returns the internal logger of the basecrawler.
//
// You may want to pass this logger to your crawler implementation manually, so that you can use the same logger (which is mostly needed if you let [CreateOptionsFromEnv] create the logger for you).
func (bc *BaseCrawler) Logger() *slog.Logger {
	return bc.logger
}

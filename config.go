package basecrawler

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
)

// MustCreateOptionsFromEnv is like [CreateOptionsFromEnv] but will panic if it encounters an error.
func MustCreateOptionsFromEnv() []CrawlerOption {
	options, err := CreateOptionsFromEnv()
	if err != nil {
		panic(err)
	}
	return options
}

// CreateOptionsFromEnv creates a collection of crawler options based on environment variables.
// It can be used by crawler implementations to configure the basecrawler and is part of this module as every crawler implementation would need to re-implement it otherwise.
//
// See the [configuration section] in the readme for available environment variables.
//
// If LOG_LEVEL is provided, this function will create a logger.
// Please note that this might overwrite the logger provided by the crawler implementation (as options are applied in order).
// To get the created logger you can use [BaseCrawler.Logger].
//
// [configuration section]: https://github.com/botnet-monitoring/basecrawler#configuration
func CreateOptionsFromEnv() ([]CrawlerOption, error) {
	var options []CrawlerOption

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel != "" {
		var level slog.Level
		if logLevel == "debug" {
			level = slog.LevelDebug
		} else if logLevel == "info" {
			level = slog.LevelInfo
		} else if logLevel == "warn" {
			level = slog.LevelWarn
		} else if logLevel == "error" {
			level = slog.LevelError
		} else {
			return nil, errors.New("log level does not exist, must be \"debug\", \"info\", \"warn\" or \"error\"")
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))
		options = append(options, WithLogger(logger))
	}

	discoveryIntervalString := os.Getenv("DISCOVERY_INTERVAL")
	trackingIntervalString := os.Getenv("TRACKING_INTERVAL")
	discoveryRemoveIntervalString := os.Getenv("DISCOVERY_REMOVE_INTERVAL")
	trackingRemoveIntervalString := os.Getenv("TRACKING_REMOVE_INTERVAL")

	var discoveryInterval uint64
	var trackingInterval uint64
	var discoveryRemoveInterval uint64
	var trackingRemoveInterval uint64
	var err error
	if discoveryIntervalString != "" {
		discoveryInterval, err = strconv.ParseUint(discoveryIntervalString, 10, 32)
		if err != nil {
			return nil, err
		}
	}
	if trackingIntervalString != "" {
		trackingInterval, err = strconv.ParseUint(trackingIntervalString, 10, 32)
		if err != nil {
			return nil, err
		}
	}
	if discoveryRemoveIntervalString != "" {
		discoveryRemoveInterval, err = strconv.ParseUint(discoveryRemoveIntervalString, 10, 32)
		if err != nil {
			return nil, err
		}
	}
	if trackingRemoveIntervalString != "" {
		trackingRemoveInterval, err = strconv.ParseUint(trackingRemoveIntervalString, 10, 32)
		if err != nil {
			return nil, err
		}
	}

	// Only checking here if we should provide that option, the basecrawler then will check the validity and set defaults
	if discoveryInterval != 0 || trackingInterval != 0 || discoveryRemoveInterval != 0 || trackingRemoveInterval != 0 {
		options = append(options, WithCustomCrawlIntervals(uint32(discoveryInterval), uint32(trackingInterval), uint32(discoveryRemoveInterval), uint32(trackingRemoveInterval)))
	}

	discoveryWorkerCountString := os.Getenv("DISCOVERY_WORKER_COUNT")
	trackingWorkerCountString := os.Getenv("TRACKING_WORKER_COUNT")
	findPeerWorkerCountString := os.Getenv("FINDPEER_WORKER_COUNT")

	var discoveryWorkerCount uint64
	var trackingWorkerCount uint64
	var findPeerWorkerCount uint64
	if discoveryWorkerCountString != "" {
		discoveryWorkerCount, err = strconv.ParseUint(discoveryWorkerCountString, 10, 32)
		if err != nil {
			return nil, err
		}
	}
	if trackingWorkerCountString != "" {
		trackingWorkerCount, err = strconv.ParseUint(trackingWorkerCountString, 10, 32)
		if err != nil {
			return nil, err
		}
	}
	if findPeerWorkerCountString != "" {
		findPeerWorkerCount, err = strconv.ParseUint(findPeerWorkerCountString, 10, 32)
		if err != nil {
			return nil, err
		}
	}

	// Only checking here if we should provide that option, the basecrawler then will check the validity and set defaults
	if discoveryWorkerCount != 0 || trackingWorkerCount != 0 || findPeerWorkerCount != 0 {
		options = append(options, WithCustomWorkerCounts(uint32(discoveryWorkerCount), uint32(trackingWorkerCount), uint32(findPeerWorkerCount)))
	}

	bmsServer := os.Getenv("BMS_SERVER")
	bmsMonitorID := os.Getenv("BMS_MONITOR")
	bmsAuthToken := os.Getenv("BMS_AUTH_TOKEN")
	bmsBotnetID := os.Getenv("BMS_BOTNET")
	bmsCampaignID := os.Getenv("BMS_CAMPAIGN")
	bmsPublicIP := os.Getenv("BMS_CRAWLER_PUBLIC_IP")

	if bmsServer != "" || bmsMonitorID != "" || bmsAuthToken != "" || bmsBotnetID != "" {
		var bmsOptions []BMSOption

		if bmsCampaignID != "" {
			bmsOptions = append(bmsOptions, WithBMSCampaign(bmsCampaignID))
		}

		if bmsPublicIP != "" {
			if net.ParseIP(bmsPublicIP) == nil {
				return nil, fmt.Errorf("ip could not be parsed: %v", bmsPublicIP)
			}
			bmsOptions = append(bmsOptions, WithBMSPublicIP(bmsPublicIP))
		}

		options = append(options, WithBMS(bmsServer, bmsMonitorID, bmsAuthToken, bmsBotnetID, bmsOptions...))
	}

	return options, nil
}

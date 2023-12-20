# BMS Basecrawler [![GoDoc][doc-img]][doc] [![Build Status][ci-img]][ci]

[doc-img]: https://pkg.go.dev/badge/github.com/botnet-monitoring/basecrawler
[doc]: https://pkg.go.dev/github.com/botnet-monitoring/basecrawler
[ci-img]: https://github.com/botnet-monitoring/basecrawler/actions/workflows/go.yml/badge.svg
[ci]: https://github.com/botnet-monitoring/basecrawler/actions/workflows/go.yml


This repository contains the implementation of a crawler framework which we call basecrawler.
You can use this crawler as a base and extend it with just the protocol implementation of a P2P botnet (see short example below).


## Example Implementation

A very shortened crawler implementation (without any error handling, logging, etc) for the testnet protocol would be:

```go
func (ci *crawlerImplementation) SendPeerRequest(ctx context.Context, config map[string]any, conn *net.UDPConn, addr *net.UDPAddr) {
	conn.WriteTo([]byte("peer-request"), addr)
}

func (ci *crawlerImplementation) ReadReply(ctx context.Context, config map[string]any, msg []byte, addr *net.UDPAddr) (basecrawler.CrawlResult, []string, []bmsclient.Edge, []bmsclient.BotReply) {
	msgParts := strings.Split(string(msg), ":")

	dstIP := net.ParseIP(msgParts[0])
	dstPort, _ := strconv.ParseUint(msgParts[1], 10, 16)

	edge := bmsclient.Edge{Timestamp: time.Now(), SrcIP: addr.IP, SrcPort: uint16(addr.Port), DstIP: dstIP, DstPort: uint16(dstPort)}
	reply := bmsclient.BotReply{Timestamp: time.Now(), IP: addr.IP, Port: uint16(addr.Port)}

	return basecrawler.BOT_REPLY, []string{string(msg)}, []bmsclient.Edge{edge}, []bmsclient.BotReply{reply}
}
```


## Configuration

The basecrawler module provides [a function](https://pkg.go.dev/github.com/botnet-monitoring/basecrawler#CreateOptionsFromEnv) that a crawler implementation can use to configure the crawler from environment variables.
The following table depicts all available variables.

| Environment Variable        | Example                                        | Description                                                                                                                                                                                                                    |
| --------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `LOG_LEVEL`                 | `info`                                         | Log level to start the crawler with. Has to be one of `debug`, `info`, `warn`, `error`. If unset, the crawler won't output anything.                                                                                           |
| `DISCOVERY_INTERVAL`        | `30`                                           | The interval (in seconds) that the crawler will wait before trying to crawl a bot again in the discovery loop. Defaults to 300s if unset.                                                                                      |
| `TRACKING_INTERVAL`         | `30`                                           | The interval (in seconds) that the crawler will wait before trying to crawl a bot again in the tracking loop. Defaults to 300s if unset.                                                                                       |
| `DISCOVERY_REMOVE_INTERVAL` | `300`                                          | The interval (in seconds) that a bot is allowed to be unresponsive or benign before the crawler will remove it from the discovery loop. Defaults to 900s if unset.                                                             |
| `TRACKING_REMOVE_INTERVAL`  | `300`                                          | The interval (in seconds) that a bot is allowed to be unresponsive or benign before the crawler will remove it from the tracking loop. Defaults to 900s if unset.                                                              |
| `DISCOVERY_WORKER_COUNT`    | `1000`                                         | The number of worker Go routines the crawler will start for crawling bots in the discovery loop. Defaults to 10000 if unset.                                                                                                   |
| `TRACKING_WORKER_COUNT`     | `1000`                                         | The number of worker Go routines the crawler will start for crawling bots in the tracking loop. Defaults to 10000 if unset.                                                                                                    |
| `FINDPEER_WORKER_COUNT`     | `10`                                           | The number of worker Go routines the crawler will start for crawling bots in the find-peer loop. Defaults to 100 if unset, will be ignored if the crawler implementation does not implement the find-peer loop.                |
| `BMS_SERVER`                | `localhost:8083`                               | The BMS server to send the crawled bot replies, edges and failed tries to. If unset, the crawler won't connect to a BMS server; has to be provided together with `BMS_MONITOR`, `BMS_AUTH_TOKEN` and `BMS_BOTNET`.             |
| `BMS_MONITOR`               | `some-monitor`                                 | The monitor ID to authenticate as. Has to exist in the database of the used BMS server. If unset, the crawler won't connect to a BMS server; has to be provided together with `BMS_SERVER`, `BMS_AUTH_TOKEN` and `BMS_BOTNET`. |
| `BMS_AUTH_TOKEN`            | `AAAA/some+example+token/AAAAAAAAAAAAAAAAAAA=` | The auth token to use for authentication. Has to be exactly 32 base64-encoded bytes. If unset, the crawler won't connect to a BMS server; has to be provided together with `BMS_SERVER`, `BMS_MONITOR` and `BMS_BOTNET`.       |
| `BMS_BOTNET`                | `some-botnet`                                  | The botnet ID to send data for. Has to exist in the database of the used BMS server. If unset, the crawler won't connect to a BMS server; has to be provided together with `BMS_SERVER`, `BMS_MONITOR` and `BMS_AUTH_TOKEN`.   |
| `BMS_CAMPAIGN`              | `some-campaign`                                | The campaign ID to send data for. Has to exist in the database of the used BMS server. Is ignored if provided without `BMS_SERVER`, `BMS_MONITOR`, `BMS_AUTH_TOKEN` and `BMS_BOTNET`.                                          |
| `BMS_CRAWLER_PUBLIC_IP`     | `198.51.100.1`                                 | The public IP address that the crawler is reachable under. Is ignored if provided without `BMS_SERVER`, `BMS_MONITOR`, `BMS_AUTH_TOKEN` and `BMS_BOTNET`.                                                                      |

All environment variables are optional.
By default the crawler will start without a BMS connection and with defaults where necessary.
Please note that especially the BMS options `BMS_SERVER`, `BMS_MONITOR`, `BMS_AUTH_TOKEN` and `BMS_BOTNET` have to be provided as a group (while `BMS_CAMPAIGN` and `BMS_CRAWLER_PUBLIC_IP` are optional, but can only used if the other four options are set).


### Peerlist

The basecrawler module also provides [a function](https://pkg.go.dev/github.com/botnet-monitoring/basecrawler#ParsePeerList) to parse a CSV file (commonly `peerlist.csv`) so that it can be passed to the basecrawler constructor.
The CSV file has just one column (IP address with port, e.g. `192.0.2.1:45678`) without header.

Since not all crawlers need an up-to-date peerlist to bootstrap the crawler, it might not be needed to provide a peerlist for a crawler.

package basecrawler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"time"

	bmsclient "github.com/botnet-monitoring/grpc-client"
)

func (bc *BaseCrawler) crawl(ctx context.Context, peer string, tries int) (CrawlResult, []string, []bmsclient.Edge, []bmsclient.BotReply, []bmsclient.FailedTry) {
	if bc.useTCP {
		return bc.crawlTCP(ctx, peer, tries)
	} else {
		return bc.crawlUDP(ctx, peer, tries)
	}
}

func (bc *BaseCrawler) crawlTCP(ctx context.Context, peer string, tries int) (CrawlResult, []string, []bmsclient.Edge, []bmsclient.BotReply, []bmsclient.FailedTry) {
	addr, _ := net.ResolveTCPAddr("tcp4", peer)
	conn, err := net.DialTCP("tcp4", nil, addr)
	if err != nil {
		bc.logger.Debug("Could not dial bot (tcp4)",
			slog.String("address", addr.String()),
			slog.Any("loop", ctx.Value(ContextKeyLoop)),
			slog.String("error", err.Error()),
		)

		failedTry := bmsclient.FailedTry{
			Timestamp: time.Now(),
			IP:        addr.IP,
			Port:      uint16(addr.Port),
			Reason:    "tcp dial error",
		}
		return NO_REPLY, []string{}, []bmsclient.Edge{}, []bmsclient.BotReply{}, []bmsclient.FailedTry{failedTry}
	}
	defer conn.Close()

	bc.tcpCrawlerImplementation.SendPeerRequest(ctx, bc.config.additionalConfig, conn, addr)
	bc.logger.Debug("Reading bot reply",
		slog.String("address", conn.RemoteAddr().String()),
		slog.Any("loop", ctx.Value(ContextKeyLoop)),
	)

	var repl bytes.Buffer
	_, err = io.Copy(&repl, conn)
	bc.logger.Debug("Received bot reply",
		slog.String("address", addr.String()),
		slog.Any("loop", ctx.Value(ContextKeyLoop)),
	)
	if err != nil {
		bc.logger.Debug("Could not read bot reply",
			slog.String("address", addr.String()),
			slog.Any("loop", ctx.Value(ContextKeyLoop)),
			slog.String("err", err.Error()),
		)

		failedTry := bmsclient.FailedTry{
			Timestamp: time.Now(),
			IP:        addr.IP,
			Port:      uint16(addr.Port),
			Reason:    "tcp read error",
		}
		return NO_REPLY, []string{}, []bmsclient.Edge{}, []bmsclient.BotReply{}, []bmsclient.FailedTry{failedTry}
	}

	bc.logger.Debug("Parsing bot reply",
		slog.String("address", addr.String()),
		slog.Any("loop", ctx.Value(ContextKeyLoop)),
		slog.Int("payload_length", repl.Len()),
	)
	msg := repl.Bytes()

	crawlResult, newPeers, edges, botReplies := bc.tcpCrawlerImplementation.ReadReply(ctx, bc.config.additionalConfig, msg, addr)
	return crawlResult, newPeers, edges, botReplies, []bmsclient.FailedTry{}
}

// Wrapper for sending and receiving peerlist request
// Has a custom retry mechanism for UDP based botnets
func (bc *BaseCrawler) crawlUDP(ctx context.Context, peer string, tries int) (CrawlResult, []string, []bmsclient.Edge, []bmsclient.BotReply, []bmsclient.FailedTry) {
	localAddr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:")
	if err != nil {
		bc.logger.Error("Could not resolve local address",
			slog.Any("loop", ctx.Value(ContextKeyLoop)),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}

	addr, _ := net.ResolveUDPAddr("udp4", peer)
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		bc.logger.Error("Could not listen on local port",
			slog.String("address", localAddr.String()),
			slog.Any("loop", ctx.Value(ContextKeyLoop)),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	defer conn.Close()

	// placeholders for crawl results
	crawlResult := NO_REPLY
	edges := []bmsclient.Edge{}
	botReplies := []bmsclient.BotReply{}
	newPeers := []string{}
	var failReason string

	repl := make([]byte, 1500) // set to 1500 byte as MTU is usually 1500

	// Sends up to "tries" botnet specific peer requests and waits for replies
	// the time waiting for a reply is doubled each time, starting from 2 seconds
	startTime := time.Now()
	for try := 1; try <= tries; try++ {
		bc.udpCrawlerImplementation.SendPeerRequest(ctx, bc.config.additionalConfig, conn, addr)

		for {
			diff := time.Since(startTime)
			timeout := time.Duration(math.Exp2(float64(try+1)))*time.Second - diff

			conn.SetReadDeadline(time.Now().Add(timeout))
			n, answeringAddr, err := conn.ReadFrom(repl)
			if err != nil {
				bc.logger.Debug("Could not read bot reply",
					slog.String("address", addr.String()),
					slog.String("error", err.Error()),
					slog.Duration("timeout", timeout),
					slog.Bool("is_timeout", os.IsTimeout(err)),
					slog.Any("loop", ctx.Value(ContextKeyLoop)),
				)
				if os.IsTimeout(err) {
					failReason = "udp timeout"
				} else {
					failReason = "udp read error"
				}
				break
			} else {
				if diff < time.Duration(timeout)*time.Second {
					if n > 0 {
						msg := repl[:n]
						crawlResult, newPeers, edges, botReplies = bc.udpCrawlerImplementation.ReadReply(ctx, bc.config.additionalConfig, msg, addr)

						if addr.String() != answeringAddr.String() {
							bc.logger.Warn("Got reply from another address as sent to",
								slog.String("requested_address", addr.String()),
								slog.String("answering_address", answeringAddr.String()),
								slog.Any("crawl_result", crawlResult),
								slog.Any("loop", ctx.Value(ContextKeyLoop)),
							)
							break
						}

						if crawlResult == BOT_REPLY {
							bc.logger.Debug("Successful bot contact",
								slog.String("bot", addr.String()),
								slog.Duration("time_to_first_response", time.Since(startTime)),
								slog.Any("loop", ctx.Value(ContextKeyLoop)),
							)
						}
						return crawlResult, newPeers, edges, botReplies, []bmsclient.FailedTry{}
					} else {
						bc.logger.Warn("Got an empty reply",
							slog.String("address", addr.String()),
							slog.Any("loop", ctx.Value(ContextKeyLoop)),
						)
						failReason = "udp empty msg"
						if addr.String() != answeringAddr.String() {
							bc.logger.Warn("Got reply from another address as sent to",
								slog.String("requested_address", addr.String()),
								slog.String("answering_address", answeringAddr.String()),
								slog.Any("crawl_result", crawlResult),
								slog.Any("loop", ctx.Value(ContextKeyLoop)),
								slog.String("fail_reason", failReason),
							)
						}
						break
					}
				} else {
					failReason = "try udp timeout"
					if addr.String() != answeringAddr.String() {
						bc.logger.Warn("Got reply from another address as sent to",
							slog.String("requested_address", addr.String()),
							slog.String("answering_address", answeringAddr.String()),
							slog.Any("crawl_result", crawlResult),
							slog.Any("loop", ctx.Value(ContextKeyLoop)),
							slog.String("fail_reason", failReason),
						)
					}
					break
				}
			}
		}
	}

	failedTry := bmsclient.FailedTry{
		Timestamp: time.Now(),
		IP:        addr.IP,
		Port:      uint16(addr.Port),
		Reason:    failReason,
	}
	return crawlResult, newPeers, edges, botReplies, []bmsclient.FailedTry{failedTry}
}

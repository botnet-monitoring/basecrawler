package basecrawler

import (
	"context"
	"net"

	bmsclient "github.com/botnet-monitoring/grpc-client"
)

// CrawlResult represents the result of a crawl attempt of a bot
type CrawlResult int

const (
	// Crawled host responded but response classified as benign
	BENIGN_REPLY CrawlResult = iota

	// Crawled host responded and response is definitely from a malicious bot
	BOT_REPLY CrawlResult = iota

	// Crawled host did not respond
	NO_REPLY CrawlResult = iota
)

type TCPBotnetCrawler interface {
	ReadReply(ctx context.Context, config map[string]any, msg []byte, addr *net.TCPAddr) (CrawlResult, []string, []bmsclient.Edge, []bmsclient.BotReply)
	SendPeerRequest(ctx context.Context, config map[string]any, conn *net.TCPConn, addr *net.TCPAddr)
}

type UDPBotnetCrawler interface {
	ReadReply(ctx context.Context, config map[string]any, msg []byte, addr *net.UDPAddr) (CrawlResult, []string, []bmsclient.Edge, []bmsclient.BotReply)
	SendPeerRequest(ctx context.Context, config map[string]any, conn *net.UDPConn, addr *net.UDPAddr)
}

type PeerFinder interface {
	SendFindPeersMsg(ctx context.Context, config map[string]any, conn *net.UDPConn, addr *net.UDPAddr)
	ReadFindPeersReply(ctx context.Context, config map[string]any, msg []byte, addr *net.UDPAddr) (CrawlResult, []string)
}

package basecrawler

import (
	"container/list"
	"context"
	"log/slog"
	"net"
	"os"
	"time"
)

func (bc *BaseCrawler) findPeers(ctx context.Context, peer string, replyChan chan string, foundPeerChan chan string, repeat int) {
	// Establish and listen on UDP connection
	localAddr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:")
	if err != nil {
		bc.logger.Error("Could not resolve local address",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	addr, _ := net.ResolveUDPAddr("udp4", peer)
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		bc.logger.Error("Could not listen on local port",
			slog.String("address", localAddr.String()),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	defer conn.Close()

	// This method only gets called when the crawler implementation implements the PeerFinder interface
	// Therefore it's ok to type cast here
	var peerFinder PeerFinder
	if bc.useTCP {
		peerFinder = bc.tcpCrawlerImplementation.(PeerFinder)
	} else {
		peerFinder = bc.udpCrawlerImplementation.(PeerFinder)
	}

	repl := make([]byte, 1500) // set to 1500 byte as MTU is usually 1500
	timeout := time.Duration(4 * time.Second)

	peerFinder.SendFindPeersMsg(ctx, bc.config.additionalConfig, conn, addr)
	conn.SetReadDeadline(time.Now().Add(timeout))
	n, _, err := conn.ReadFrom(repl)
	if err != nil {
		bc.logger.Debug("Could not read find peers msg",
			slog.String("address", addr.String()),
			slog.String("error", err.Error()),
			slog.Duration("timeout", timeout),
		)
		return
	}

	if n <= 0 {
		bc.logger.Debug("Received message is empty",
			slog.String("address", addr.String()),
		)
		return
	}

	msg := repl[:n]
	crawlResult, newPeers := peerFinder.ReadFindPeersReply(ctx, bc.config.additionalConfig, msg, addr)
	if crawlResult == BOT_REPLY {
		for _, p := range newPeers {
			replyChan <- p
		}
		foundPeerChan <- peer
	} else {
		for _, p := range newPeers {
			foundPeerChan <- p
		}
	}
}

func (bc *BaseCrawler) findPeerWorker(ctx context.Context, findPeerChan chan string, foundPeerChan chan string, replyChan chan string, workerID int) {
	ctx = context.WithValue(ctx, ContextKeyWorkerID, workerID)

outerLoop:
	for {
		select {
		case b := <-findPeerChan:
			bc.findPeers(ctx, b, replyChan, foundPeerChan, 1)

			time.Sleep(100 * time.Millisecond)

		case <-ctx.Done():
			break outerLoop

		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (bc *BaseCrawler) findPeerLoop(ctx context.Context, replyChan chan string) {
	ctx = context.WithValue(ctx, ContextKeyLoop, "find_peer_loop")

	findNodeQueue := list.New()

	findPeerChan := make(chan string, 1000)
	foundPeerChan := make(chan string, 1000)

	knownNodes := make(map[string]struct{})

	for i := 0; i < int(bc.config.findPeerWorkerCount); i++ {
		go bc.findPeerWorker(ctx, findPeerChan, foundPeerChan, replyChan, i)
	}

	// Parse and push the bootstrap peers into the discovery queue
	bootstrapPeers := bc.bootstrapPeers
	for ipPort := range bootstrapPeers {
		if !bc.IsBlacklisted(ipPort) {
			findNodeQueue.PushBack(ipPort)
		}
		knownNodes[ipPort] = struct{}{}
	}

	logTimer := time.Now()

outerLoop:
	for {
		select {
		case reply := <-foundPeerChan: // Parse replies for new peers first
			if reply != "" {
				_, ok := knownNodes[reply]
				if !ok {
					knownNodes[reply] = struct{}{}
					if !bc.IsBlacklisted(reply) {
						findNodeQueue.PushBack(reply)
					}
				}
			}

		case <-ctx.Done():
			break outerLoop

		default: // Crawl another peer
			potentialNewBot := findNodeQueue.Front()
			if potentialNewBot != nil {
				findPeerChan <- potentialNewBot.Value.(string)
				findNodeQueue.Remove(potentialNewBot)
				delete(knownNodes, potentialNewBot.Value.(string))
				time.Sleep(100 * time.Millisecond)
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}

		if time.Since(logTimer) > 10*time.Second {
			bc.logger.Info("FindPeer loop stats",
				slog.Int("queue_length", findNodeQueue.Len()),
				slog.Int("worker_chan_size", len(findPeerChan)),
				slog.Int("foundPeerChan_size", len(foundPeerChan)),
			)
			logTimer = time.Now()
		}
	}
}

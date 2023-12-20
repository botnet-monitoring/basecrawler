package basecrawler

import (
	"container/list"
	"context"
	"log/slog"
	"time"
)

// Worker for the discovery loop
// Interprets and acts upon the results of calling Crawl() at the stage of the discovery loop
func (bc *BaseCrawler) discoveryWorker(ctx context.Context, discoveryWorkerChan chan *bot, replyChan chan string, discoveryLoopChan chan *bot, trackingLoopChan chan *bot, workerID int) {
	ctx = context.WithValue(ctx, ContextKeyWorkerID, workerID)

outerLoop:
	for {
		select {
		case potentialNewBot := <-discoveryWorkerChan:
			crawlResult, replies, _, _, _ := bc.crawl(ctx, potentialNewBot.IpPort, 1)

			if crawlResult == BOT_REPLY {
				bc.logger.Debug("Moving bot from discovery to tracking loop",
					slog.String("bot", potentialNewBot.IpPort),
				)
				potentialNewBot.LastCrawled = time.Now()
				potentialNewBot.LastSeen = time.Now()
				trackingLoopChan <- potentialNewBot
				for _, reply := range replies {
					replyChan <- reply
				}
			} else if crawlResult == BENIGN_REPLY {
				bc.logger.Debug("Got reply from benign peer, pushing to discovery loop again",
					slog.String("bot", potentialNewBot.IpPort),
				)

				potentialNewBot.LastCrawled = time.Now()
				for _, reply := range replies {
					replyChan <- reply
				}
				discoveryLoopChan <- potentialNewBot
			} else {
				bc.logger.Debug("Didn't reach bot, moving into loop again",
					slog.String("bot", potentialNewBot.IpPort),
				)
				potentialNewBot.LastCrawled = time.Now()
				discoveryLoopChan <- potentialNewBot
			}

			time.Sleep(100 * time.Millisecond)

		case <-ctx.Done():
			break outerLoop

		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Loop for discovering peers.
// This loop works without retries, tries to find stuff we don't know.
// Peers come back here before they finally rest in peace (get dropped for not replying too long)
func (bc *BaseCrawler) discoveryLoop(ctx context.Context, trackingLoopChan chan *bot, replyChan chan string, discoveryLoopChan chan *bot) {
	ctx = context.WithValue(ctx, ContextKeyLoop, "discovery_loop")

	knownPeers := make(map[string]*bot) // Known peers used by the crawler
	discoveryQueue := list.New()

	// Parse and push the bootstrap peers into the discovery queue
	if !bc.useFindPeerLoop {
		bootstrapPeers := bc.bootstrapPeers
		bc.logger.Debug("Loaded bootstrap peers",
			slog.Int("peer_count", len(bc.bootstrapPeers)),
		)
		for key := range bootstrapPeers {
			newBot := bot{key, time.Time{}, time.Time{}}
			knownPeers[key] = &newBot
			if !bc.IsBlacklisted(newBot.IpPort) {
				discoveryQueue.PushBack(&newBot)
			}
		}
	}

	discoveryWorkerChan := make(chan *bot, 100000)

	for i := 0; i < int(bc.config.discoveryWorkerCount); i++ {
		go bc.discoveryWorker(ctx, discoveryWorkerChan, replyChan, discoveryLoopChan, trackingLoopChan, i)
	}

	logTimer := time.Now()

outerLoop:
	for {
		select {
		case reply := <-replyChan: // Parse replies for new peers first
			if reply != "" {
				_, ok := knownPeers[reply]
				if !ok {
					bc.logger.Debug("New peer discovered",
						slog.String("bot", reply),
					)
					newBot := bot{reply, time.Time{}, time.Time{}}
					knownPeers[reply] = &newBot
					if !bc.IsBlacklisted(newBot.IpPort) {
						discoveryQueue.PushFront(&newBot)
					}
				}
			}

		case bot := <-discoveryLoopChan: // Push peers back into the queue after they have been crawled
			discoveryQueue.PushBack(bot)

		case <-ctx.Done():
			break outerLoop

		default: // Crawl another peer
			botToCrawl := discoveryQueue.Front()
			if botToCrawl != nil {
				if botToCrawl.Value.(*bot).LastCrawled.IsZero() {
					// If a bot wasn't ever crawled, crawl it
					discoveryWorkerChan <- botToCrawl.Value.(*bot)
					discoveryQueue.Remove(botToCrawl)
				} else if time.Since(botToCrawl.Value.(*bot).LastSeen) > time.Duration(bc.config.discoveryRemoveInterval)*time.Second {
					// If a bot wasn't seen longer than discoveryRemoveInterval ago, remove it
					discoveryQueue.Remove(botToCrawl)
					delete(knownPeers, botToCrawl.Value.(*bot).IpPort)
				} else if time.Since(botToCrawl.Value.(*bot).LastCrawled) > time.Duration(bc.config.discoveryInterval)*time.Second {
					// If a bot wasn't crawled longer than discoveryInterval ago, crawl it
					discoveryWorkerChan <- botToCrawl.Value.(*bot)
					discoveryQueue.Remove(botToCrawl)
				} else {
					time.Sleep(100 * time.Millisecond)
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}

		if time.Since(logTimer) > 10*time.Second {
			bc.logger.Info("Discovery loop stats",
				slog.Int("queue_length", discoveryQueue.Len()),
				slog.Int("worker_chan_size", len(discoveryWorkerChan)),
				slog.Int("replyChan_size", len(replyChan)),
			)
			logTimer = time.Now()
		}
	}
}

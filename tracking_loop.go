package basecrawler

import (
	"container/list"
	"context"
	"log/slog"
	"time"
)

// Worker for the tracking loop
// Sends up to 5 messages in total with waiting times of 2,4,8,16, and 32 seconds
// Replies are stored to DB & BMS
// Upon failure, bots are moved back to discovery loop
func (bc *BaseCrawler) trackingWorker(ctx context.Context, trackingWorkerChan chan *bot, replyChan chan string, trackingLoopChan chan *bot, discoveryLoopChan chan *bot, workerID int) {
	ctx = context.WithValue(ctx, ContextKeyWorkerID, workerID)

outerLoop:
	for {
		select {
		case b := <-trackingWorkerChan:
			bc.logger.Debug("Crawling bot",
				slog.Int("worker_id", workerID),
				slog.String("bot", b.IpPort),
			)

			crawlResult, replies, edges, datedBotReplies, datedFailedTries := bc.crawl(ctx, b.IpPort, 4)
			b.LastCrawled = time.Now()
			if crawlResult == BOT_REPLY {
				b.LastSeen = time.Now()
			}

			if crawlResult != NO_REPLY {
				if bc.withBMS && bc.activeSession {
					for _, edge := range edges {
						bc.bmsSession.AddEdgeStruct(&edge)
					}

					for _, datedBotReply := range datedBotReplies {
						bc.bmsSession.AddBotReplyStruct(&datedBotReply)
					}
				}

				for _, reply := range replies {
					replyChan <- reply
				}

				// Note: The following lines are commented out because they currently aren't needed
				// In theory there could be failed tries when crawlResult != NO_REPLY (e.g. in case the implemented SendPeerRequest implements retries itself)
				// Re-enable this when it gets needed at some point in the future
				// for _, datedFailedTry := range datedFailedTries {
				// 	bc.FailedTryChannel <- datedFailedTry
				// }

				trackingLoopChan <- b
			} else {
				if bc.withBMS && bc.activeSession {
					for _, datedFailedTry := range datedFailedTries {
						bc.bmsSession.AddFailedTryStruct(&datedFailedTry)
					}
				}

				if time.Since(b.LastSeen) > time.Duration(bc.config.trackingRemoveInterval)*time.Second {
					discoveryLoopChan <- b
				} else {
					trackingLoopChan <- b
				}
			}

			time.Sleep(100 * time.Millisecond)

		case <-ctx.Done():
			break outerLoop

		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Tracking loop
func (bc *BaseCrawler) trackingLoop(ctx context.Context, trackingLoopChan chan *bot, replyChan chan string, discoveryLoopChan chan *bot) { //conn *net.UDPConn, session *bmsclient.Session, baseCrawler BotnetCrawler) {
	ctx = context.WithValue(ctx, ContextKeyLoop, "tracking_loop")

	trackingQueue := list.New()

	trackingWorkerChan := make(chan *bot, 100000)

	for i := 0; i < int(bc.config.trackingWorkerCount); i++ {
		go bc.trackingWorker(ctx, trackingWorkerChan, replyChan, trackingLoopChan, discoveryLoopChan, i)
	}

	logTimer := time.Now()

outerLoop:
	for {
		select {
		case botToRequeue := <-trackingLoopChan:
			if !bc.IsBlacklisted(botToRequeue.IpPort) {
				trackingQueue.PushBack(botToRequeue)
			}

		case <-ctx.Done():
			break outerLoop

		default:
			botToCrawl := trackingQueue.Front()
			if botToCrawl != nil {
				if time.Since(botToCrawl.Value.(*bot).LastCrawled) > time.Duration(bc.config.trackingInterval)*time.Second {
					trackingWorkerChan <- botToCrawl.Value.(*bot)
					trackingQueue.Remove(botToCrawl)
				}
			}
			time.Sleep(100 * time.Millisecond)

		}

		if time.Since(logTimer) > 10*time.Second {
			bc.logger.Info("Tracking loop stats",
				slog.Int("queue_length", trackingQueue.Len()),
				slog.Int("worker_chan_size", len(trackingWorkerChan)),
			)
			logTimer = time.Now()
		}
	}
}

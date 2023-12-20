package basecrawler_test

import (
	"context"
	"time"

	"github.com/botnet-monitoring/basecrawler"
)

func ExampleBaseCrawler_Start_cancelContext() {
	// Providing this to NewCrawler actually will result in an error since it neither implements TCPBotnetCrawler nor UDPBotnetCrawler
	someProperImplementation := struct{}{}

	crawler, err := basecrawler.NewCrawler(
		someProperImplementation,
		[]string{
			"192.0.2.1:20001",
			"192.0.2.2:20002",
			"192.0.2.3:20003",
		},
	)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	crawler.Start(ctx)
	time.Sleep(5 * time.Second)
	cancel()

	// Do other stuff
}

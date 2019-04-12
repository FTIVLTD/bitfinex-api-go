package main

import (
	"context"
	_ "net/http/pprof"

	"github.com/FTIVLTD/bitfinex-api-go/v2"
	"github.com/FTIVLTD/bitfinex-api-go/v2/websocket"
	logger "github.com/FTIVLTD/logger/src"
)

func main() {
	// create a new logger instance
	log, err := logger.InitLogger("bfx-websocket", "debug", "", logger.Ltimestamp|logger.LJSON)
	if err != nil {
		log.Fatalf("InitLogger() error = %s", err.Error())
	}

	// create websocket client and pass logger
	p := websocket.NewDefaultParameters()
	p.Logger = log
	client := websocket.NewWithParams(p)
	err = client.Connect()
	if err != nil {
		log.Errorf("could not connect: %s", err.Error())
		return
	}

	for obj := range client.Listen() {
		switch obj.(type) {
		case error:
			log.Errorf("channel closed: %s", obj)
			return
		case *bitfinex.Trade:
			log.Infof("New trade: %s", obj)
		case *websocket.InfoEvent:
			// Info event confirms connection to the bfx websocket
			log.Info("Subscribing to tBTCUSD")
			_, err := client.SubscribeTrades(context.Background(), "tBTCUSD")
			if err != nil {
				log.Infof("could not subscribe to trades: %s", err.Error())
			}
		default:
			log.Infof("MSG RECV: %#v", obj)
		}
	}
}

package websocket

import (
	"log"
	"mmrobot/gateway/common"
	"time"

	logger "github.com/FTIVLTD/logger/src"
)

// Parameters defines adapter behavior.
type Parameters struct {
	AutoReconnect     bool
	ReconnectInterval time.Duration
	ReconnectAttempts int
	reconnectTry      int
	ShutdownTimeout   time.Duration
	Logger            *logger.LoggerType

	ResubscribeOnReconnect bool

	HeartbeatTimeout time.Duration
	LogTransport     bool

	URL             string
	ManageOrderbook bool
}

func NewDefaultParameters(name, logLevel string) *Parameters {
	logger, err := common.InitLogger(name)
	if err != nil {
		log.Fatalf("NewDefaultParameters() error = %s", err.Error())
	}
	logger.SetLogLevel(logLevel)
	return &Parameters{
		AutoReconnect:          true,
		ReconnectInterval:      time.Second * 3,
		reconnectTry:           0,
		ReconnectAttempts:      15,
		URL:                    productionBaseURL,
		ManageOrderbook:        false,
		ShutdownTimeout:        time.Second * 5,
		ResubscribeOnReconnect: true,
		HeartbeatTimeout:       time.Second * 30,
		LogTransport:           false, // log transport send/recv
		Logger:                 logger,
	}
}

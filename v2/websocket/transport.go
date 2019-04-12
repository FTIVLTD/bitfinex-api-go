package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/op/go-logging"

	"github.com/gorilla/websocket"
)

func newWs(baseURL string, logTransport bool, log *logging.Logger) *ws {
	return &ws{
		BaseURL:      baseURL,
		downstream:   make(chan []byte),
		quit:         make(chan error),
		logTransport: logTransport,
		log:          log,
	}
}

type ws struct {
	ws            *websocket.Conn
	wsLock        sync.Mutex
	BaseURL       string
	TLSSkipVerify bool
	downstream    chan []byte
	logTransport  bool
	log           *logging.Logger

	quit chan error // signal to parent with error, if applicable
}

func (w *ws) Connect() error {
	if w.ws != nil {
		return nil // no op
	}
	w.wsLock.Lock()
	defer w.wsLock.Unlock()
	var d = websocket.Dialer{
		Subprotocols:    []string{"p1", "p2"},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Proxy:           http.ProxyFromEnvironment,
	}

	d.TLSClientConfig = &tls.Config{InsecureSkipVerify: w.TLSSkipVerify}

	w.log.Info("connecting ws to %s", w.BaseURL)
	ws, resp, err := d.Dial(w.BaseURL, nil)
	if err != nil {
		if err == websocket.ErrBadHandshake {
			w.log.Errorf("bad handshake: status code %d", resp.StatusCode)
		}
		return err
	}
	w.ws = ws
	go w.listenWs()
	return nil
}

// Send marshals the given interface and then sends it to the API. This method
// can block so specify a context with timeout if you don't want to wait for too
// long.
func (w *ws) Send(ctx context.Context, msg interface{}) error {
	if w.ws == nil {
		return ErrWSNotConnected
	}

	bs, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.quit: // ws closed
		return fmt.Errorf("websocket connection closed")
	default:
	}

	w.wsLock.Lock()
	defer w.wsLock.Unlock()
	w.log.Debug("ws->srv: %s", string(bs))
	err = w.ws.WriteMessage(websocket.TextMessage, bs)
	if err != nil {
		return err
	}
	return nil
}

func (w *ws) Done() <-chan error {
	return w.quit
}

// listen on ws & fwd to listen()
func (w *ws) listenWs() {
	for {
		if w.ws == nil {
			return
		}

		select {
		case <-w.quit: // ws connection ended
			return
		default:
			_, msg, err := w.ws.ReadMessage()
			if err != nil {
				if cl, ok := err.(*websocket.CloseError); ok {
					w.log.Errorf("close error code: %d", cl.Code)
				}
				// a read during normal shutdown results in an OpError: op on closed connection
				if _, ok := err.(*net.OpError); ok {
					// general read error on a closed network connection, OK
					return
				}
				w.cleanup(err)
				return
			}
			w.log.Debugf("srv->ws: %s", string(msg))
			w.downstream <- msg
		}
	}
}

func (w *ws) Listen() <-chan []byte {
	return w.downstream
}

func (w *ws) cleanup(err error) {
	w.stop()
	w.quit <- err       // pass error back
	close(w.quit)       // signal to parent listeners
	close(w.downstream) // shut down caller's listen channel
}

func (w *ws) stop() {
	w.wsLock.Lock()
	if w.ws != nil {
		if err := w.ws.Close(); err != nil { // will trigger cleanup()
			w.log.Errorf("error closing websocket: %s", err)
		}
		w.ws = nil
	}
	w.wsLock.Unlock()
}

// Close the websocket connection
func (w *ws) Close() {
	w.stop()
}

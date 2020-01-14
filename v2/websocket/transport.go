package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	logger "github.com/FTIVLTD/logger/src"
	"github.com/gorilla/websocket"
)

func newWs(baseURL string, logTransport bool, log *logger.LoggerType) *ws {
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
	log           *logger.LoggerType

	quit chan error // signal to parent with error, if applicable
}

func (w *ws) Connect() error {
	w.wsLock.Lock()
	defer w.wsLock.Unlock()
	if w.ws != nil {
		return nil // no op
	}
	var d = websocket.Dialer{
		Subprotocols:    []string{"p1", "p2"},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Proxy:           http.ProxyFromEnvironment,
	}

	d.TLSClientConfig = &tls.Config{InsecureSkipVerify: w.TLSSkipVerify}

	w.log.Infof("connecting to %s", w.BaseURL)
	ws, resp, err := d.Dial(w.BaseURL, nil)
	if err != nil {
		if err == websocket.ErrBadHandshake {
			w.log.Errorf("bad handshake: status code %d", resp.StatusCode)
		}
		close(w.quit)       // signal to parent listeners
		close(w.downstream) // shut down caller's listen channel
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
	w.wsLock.Lock()
	if w.ws == nil {
		return ErrWSNotConnected
	}
	w.wsLock.Unlock()

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
	curWs := w.ws
	w.wsLock.Unlock()

	if curWs == nil {
		return ErrWSNotConnected
	}
	w.log.Debugf("ws->srv: %s", string(bs))
	// TODO: Not sure should this call be under the mutex lock above
	// But it looks like it doesn't
	err = curWs.WriteMessage(websocket.TextMessage, bs)
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
	w.log.Infof("go listenWs() START")
	defer w.log.Infof("go listenWs() FINISH")
	for {
		w.wsLock.Lock()
		curWs := w.ws
		w.wsLock.Unlock()

		if curWs == nil {
			return
		}

		_, msg, err := curWs.ReadMessage()
		if err != nil {
			if cl, ok := err.(*websocket.CloseError); ok {
				w.log.Errorf("close error code: %d", cl.Code)
			}
			// a read during normal shutdown results in an OpError: op on closed connection
			if _, ok := err.(*net.OpError); ok {
				// general read error on a closed network connection, OK
				w.log.Errorf("go listenWs: general read error on a closed network connection")
				//return
			}
			w.cleanup(err) // cleanup and notify
			return
		}
		w.log.Debugf("srv->ws: %s", string(msg))
		w.downstream <- msg
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
	// TODO: Avoid call Close and Errorf under the mutex lock
	defer w.wsLock.Unlock()
	if w.ws != nil {
		if err := w.ws.Close(); err != nil { // will trigger cleanup()
			w.log.Errorf("error closing websocket: %s", err)
		}
		w.ws = nil
	}
}

// Close the websocket connection
func (w *ws) Close() {
	w.stop()
}

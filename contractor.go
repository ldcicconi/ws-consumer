package wscontractor

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

var (
	AlreadyConsumingError = fmt.Errorf("Already consuming")
)

type WsContractor struct {
	wsURL               *url.URL
	dialer              websocket.Dialer
	subscriptionMessage []byte
	reconnectChan       chan bool
	killChan            chan bool
	consuming           bool // Or connecting
}

type MessageEnvelope struct {
	Payload          []byte
	ReceiptTimestamp time.Time
}

func NewWsContractor(URL url.URL, subMessage []byte, isSecure bool) *WsContractor {
	if subMessage == nil {
		return nil
	}
	dialer := *websocket.DefaultDialer
	if isSecure {
		dialer = websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &WsContractor{
		wsURL:               &URL,
		subscriptionMessage: subMessage,
		reconnectChan:       make(chan bool), //If the websocket connection is interupted, the consumer closes the connection and signals that it needs to be restarted
		killChan:            make(chan bool),
		dialer:              dialer,
	}
}

func (ctrctr *WsContractor) Consume(outputChan chan MessageEnvelope) error {
	if ctrctr.consuming {
		return AlreadyConsumingError
	}
	// this starts the whole thing
	go func() {
		ctrctr.reconnectChan <- true
	}()
	// spin off routine that loops forever
	go func() {
		for {
			select {
			case <-ctrctr.reconnectChan:
				ws, success := ctrctr.connectAndSubscribe()
				if success {
					// Consume from the websocket
					go ctrctr.consumeUntilDisconect(ws, outputChan)
				} else {
					log.Println("failure")
					ctrctr.reconnectChan <- true
				}
			case <-ctrctr.killChan:
				ctrctr.consuming = false
			}
		}
	}()
	return nil
}

func (ctrctr *WsContractor) Kill() {
	ctrctr.consuming = false
	ctrctr.killChan <- true
}

func (ctrctr *WsContractor) connectAndSubscribe() (conn *websocket.Conn, success bool) {
	// Connect
	log.Printf("connecting to %s", ctrctr.wsURL.String())
	ws, _, err := ctrctr.dialer.Dial(ctrctr.wsURL.String(), nil)
	if err != nil {
		ws.Close()
		log.Fatal("dial:", err)
		return nil, false
	}
	// Subscribe
	log.Printf("submessage: %s", string(ctrctr.subscriptionMessage))
	err = ws.WriteMessage(websocket.TextMessage, ctrctr.subscriptionMessage)
	if err != nil {
		log.Fatal("subsr:", err)
		ws.Close()
		return nil, false
	}
	return ws, true
}

func (ctrctr *WsContractor) consumeUntilDisconect(ws *websocket.Conn, outputChan chan MessageEnvelope) {
	defer ws.Close()
	for {
		// Read message
		_, message, err := ws.ReadMessage()
		now := time.Now()
		// TODO: better error handling
		if err != nil {
			log.Println("read:", err)
			ctrctr.reconnectChan <- true
			return
		}
		// Handle message
		outputChan <- MessageEnvelope{Payload: message, ReceiptTimestamp: now}
	}
}

package wscontractor

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"

	"github.com/gorilla/websocket"
)

var (
	AlreadyConsumingError = fmt.Errorf("Already consuming")
)

type WsContractor struct {
	wsURL               *url.URL
	dialer              websocket.Dialer
	subscriptionMessage []byte
	messageHandler      func([]byte)
	isPipeline          bool
	dataChan            chan []byte
	reconnectChan       chan bool
	killChan            chan bool
	consuming           bool // Or connecting
}

func NewWsContractor(URL url.URL, subMessage []byte, messageHandlerFunc func([]byte), isSecure bool) *WsContractor {
	if subMessage == nil || messageHandlerFunc == nil {
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
		messageHandler:      messageHandlerFunc,
		reconnectChan:       make(chan bool), //If the websocket connection is interupted, the consumer closes the connection and signals that it needs to be restarted
		killChan:            make(chan bool),
		dialer:              dialer,
	}
}

func NewWsContractorAsPipeline(URL url.URL, subMessage []byte, dataChan chan []byte, isSecure bool) *WsContractor {
	contractor := NewWsContractor(URL, subMessage, nil, isSecure)
	contractor.dataChan = dataChan
	contractor.isPipeline = true
	return contractor
}

func (ctrctr *WsContractor) Consume() error {
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
					log.Println("success")
					go ctrctr.consumeUntilDisconect(ws)
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

func (ctrctr *WsContractor) consumeUntilDisconect(ws *websocket.Conn) {
	defer ws.Close()
	for {
		// Read message
		_, message, err := ws.ReadMessage()
		// TODO: better error handling
		if err != nil {
			log.Println("read:", err)
			ctrctr.reconnectChan <- true
			return
		}
		// Handle message
		if ctrctr.isPipeline {
			ctrctr.dataChan <- message
		} else {
			go ctrctr.messageHandler(message)
		}
	}
}

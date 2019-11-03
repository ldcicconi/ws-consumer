package wscontractor

import (
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
	subscriptionMessage []byte
	messageHandler      func([]byte)
	reconnectChan       chan bool
	killChan            chan bool
	consuming           bool // Or connecting
}

func NewWsContractor(URL string, subMessage []byte, messageHandlerFunc func([]byte), isSecure bool) *WsContractor {
	if URL == "" || subMessage == nil || messageHandlerFunc == nil {
		return nil
	}
	scheme := "ws"
	if isSecure {
		scheme = "wss"
	}
	return &WsContractor{
		wsURL: &url.URL{
			Scheme: scheme,
			Host:   URL,
			Path:   "/",
		},
		subscriptionMessage: subMessage,
		messageHandler:      messageHandlerFunc,
		reconnectChan:       make(chan bool), //If the websocket connection is interupted, the consumer closes the connection and signals that it needs to be restarted
		killChan:            make(chan bool),
	}
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
	ws, _, err := websocket.DefaultDialer.Dial("wss://ws-feed.pro.coinbase.com/", nil)
	log.Printf("dialed")
	if err != nil {
		ws.Close()
		log.Fatal("dial:", err)
		return nil, false
	}
	// Subscribe

	err = ws.WriteMessage(websocket.BinaryMessage, ctrctr.subscriptionMessage)
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
		if ctrctr.consuming == false {
			return
		}
		// Read message
		fmt.Println("reading message")
		_, message, err := ws.ReadMessage()
		// TODO: better error handling
		if err != nil {
			log.Println("read:", err)
			ctrctr.reconnectChan <- true
			return
		}
		// Handle message
		go ctrctr.messageHandler(message)
	}
}

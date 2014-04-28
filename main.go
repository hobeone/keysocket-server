//
// Simple daemon to grab keys and then send them via websocket to the
// keysocket Chrome plugin.
//
// Written by Daniel Hobe but based off of many code examples found online.
//
// Websocket server: garyburd
// https://gist.github.com/1316852
//
// keybinding example: BurtSushi
// https://github.com/BurntSushi/xgbutil/blob/master/examples/simple-keybinding/main.go

//
// Future features:
// - config file
// - connect to Gnome setting daemon over dbus

package main

import (
	"flag"
	"log"
	"net/http"

	"code.google.com/p/go.net/websocket"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/BurntSushi/xgbutil/xevent"
)

type websocket_connections struct {
	connections map[*connection]bool
	broadcast   chan string
	register    chan *connection
	unregister  chan *connection
}

var h = websocket_connections{
	broadcast:   make(chan string),
	register:    make(chan *connection),
	unregister:  make(chan *connection),
	connections: make(map[*connection]bool),
}

func (h *websocket_connections) run() {
	for {
		select {
		case c := <-h.register:
			h.connections[c] = true
		case c := <-h.unregister:
			delete(h.connections, c)
			close(c.send)
		case m := <-h.broadcast:
			log.Printf("Sending %q\n", m)
			for c := range h.connections {
				select {
				case c.send <- m:
				default:
					delete(h.connections, c)
					close(c.send)
					go c.ws.Close()
				}
			}
		}
	}
}

type connection struct {
	// The websocket connection.
	ws *websocket.Conn

	// Buffered channel of outbound messages.
	send chan string
}

func (c *connection) reader() {
	for {
		var message string
		err := websocket.Message.Receive(c.ws, &message)
		if err != nil {
			break
		}
		if message == "Ping" {
			err := websocket.Message.Send(c.ws, "Pong")
			if err != nil {
				break
			}
		}
	}
	c.ws.Close()
}

func (c *connection) writer() {
	for message := range c.send {
		err := websocket.Message.Send(c.ws, message)
		if err != nil {
			break
		}
	}
	c.ws.Close()
}

func wsHandler(ws *websocket.Conn) {
	c := &connection{send: make(chan string, 2), ws: ws}
	h.register <- c
	defer func() { h.unregister <- c }()
	go c.writer()
	c.reader()
}

func BindKeys(keymap map[string]string) {
	X, err := xgbutil.NewConn()
	if err != nil {
		log.Fatal(err)
	}
	keybind.Initialize(X)
	for k, v := range keymap {
		v := v
		err = keybind.KeyPressFun(func(X *xgbutil.XUtil, e xevent.KeyPressEvent) {
			h.broadcast <- v
		}).Connect(X, X.RootWin(), k, true)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("Program initialized. Start pressing keys!")
	xevent.Main(X)
}

var addr = flag.String("addr", "localhost:1337", "http service address")
var next_key = flag.String("next_key", "XF86AudioNext", "Key to skip to next track.")
var prev_key = flag.String("prev_key", "XF86AudioPrev", "Key to skip to previous track.")
var play_key = flag.String("play_key", "XF86AudioPlay", "Key to play .")
var pause_key = flag.String("pause_key", "XF86AudioPause", "Key to pause.")

func main() {
	flag.Parse()
	// Maybe make this flag or config file settable.
	var keymap = map[string]string{
		*next_key:  "19", // next
		*prev_key:  "20", // prev
		*play_key:  "16", // play
		*pause_key: "16", // pause
	}

	go h.run()
	go BindKeys(keymap)
	http.Handle("/", websocket.Handler(wsHandler))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

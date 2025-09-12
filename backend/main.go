package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

func blue(s string) string {
	return "\x1b[36m" + s + "\x1b[m"
}

type handler struct{}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

var fire *websocket.Conn = nil
var chro *websocket.Conn = nil

type Msg struct {
	Type      string `json:"type"`
	Sdp       string `json:"sdp,omitempty"`
	Candidate string `json:"new-ice-candidate,omitempty"`
}

type Answer struct {
	Answer webrtc.SessionDescription `json:"answer"`
}

func mediaHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Upgrade error:", err)
		return
	}
	defer ws.Close()

	isChrome := strings.Contains(r.UserAgent(), "Chrome")

	if isChrome {
		chro = ws
	} else {
		fire = ws
	}

	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})

	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}

		candidateFull, _ := json.Marshal(c.ToJSON())

		b, _ := json.Marshal(Msg{
			Type:      "candidate",
			Candidate: string(candidateFull),
		})
		ws.WriteMessage(websocket.TextMessage, b)
	})

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			fmt.Println("Read error:", err)
			break
		}

		// fmt.Printf("msg: %s, t: %d\n", data, t)

		var msg Msg
		err = json.Unmarshal(data, &msg)
		if err != nil {
			fmt.Println("Unmarshal error:", err)
			break
		}

		if msg.Type == "offer" {
			peer.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.Sdp,
			})

			answer, err := peer.CreateAnswer(&webrtc.AnswerOptions{})
			if err != nil {
				fmt.Println("create answer error:", err)
				break
			}

			peer.SetLocalDescription(answer)

			ws.WriteJSON(answer)
		}

		if msg.Type == "new-ice-candidate" {
			peer.AddICECandidate(webrtc.ICECandidateInit{Candidate: msg.Candidate})
		}

		// var writeWS *websocket.Conn
		// if isChrome {
		// 	writeWS = fire
		// } else {
		// 	writeWS = chro
		// }

		// if err := writeWS.WriteMessage(t, msg); err != nil {
		// 	fmt.Println("Write error:", err)
		// 	break
		// }

	}
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	switch r.URL.Path {
	case "/api/media":
		mediaHandler(w, r)
		return
	}

	if r.Method == "GET" {
		w.Write([]byte("back"))
		return
	}

	b, _ := io.ReadAll(r.Body)
	w.Write([]byte("POST back, " + string(b)))
}

func main() {
	listener, err := net.Listen("tcp", "0.0.0.0:8081")
	if err != nil {
		fmt.Println(err)
		return
	}

	handler := handler{}

	fmt.Println(blue("Serving"))

	err = http.Serve(listener, handler)
	if err != nil {
		fmt.Println(err)
		return
	}
}

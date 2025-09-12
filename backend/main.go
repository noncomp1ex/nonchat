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

type Peer struct {
	peer *webrtc.PeerConnection
	ws   *websocket.Conn
}

var peers []Peer = make([]Peer, 0)

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

	fmt.Println("ws from", r.Host)

	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	peers = append(peers, Peer{peer: peer, ws: ws})

	peer.OnTrack(func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		fmt.Println("Got track:", track.Kind())

		for _, other := range peers {
			if other.peer == peer && other.peer != nil {
				continue // don't forward to the same peer
			}
			if other.peer.ConnectionState() == webrtc.PeerConnectionStateClosed {
				continue
			}
			fmt.Println("found other")

			// Create a local track to send to the other peer
			localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, "audio", "sfu")
			if err != nil {
				panic(err)
			}

			sender, err := other.peer.AddTrack(localTrack)
			if err != nil {
				panic(err)
			}

			// offer
			{
				offer, err := other.peer.CreateOffer(nil)
				if err != nil {
					fmt.Println(err)
					continue
				}
				err = other.peer.SetLocalDescription(offer)
				if err != nil {
					fmt.Println(err)
					continue
				}

				jsonOffer, _ := json.Marshal(Msg{Type: "offer", Sdp: offer.SDP})
				other.ws.WriteMessage(websocket.TextMessage, jsonOffer)
			}

			// Forward RTP packets from remote track to local track
			go func() {
				buf := make([]byte, 1500)
				for {
					n, _, err := track.Read(buf)
					if err != nil {
						return
					}
					if _, err := localTrack.Write(buf[:n]); err != nil {
						return
					}
				}
			}()

			// Optional: handle RTCP packets (ACKs, jitter reports)
			go func() {
				rtcpBuf := make([]byte, 1500)
				for {
					if _, _, err := sender.Read(rtcpBuf); err != nil {
						return
					}
				}
			}()
		}
	})

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

		if msg.Type == "answer" {
			peer.SetRemoteDescription(
				webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  msg.Sdp,
				})
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

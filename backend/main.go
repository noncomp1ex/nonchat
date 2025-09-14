package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

func blue(s string) string {
	return "\x1b[36m" + s + "\x1b[m"
}

const (
	PORT = 8081
)

type handler struct{}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type Peer struct {
	peerConn *webrtc.PeerConnection
	ws       *websocket.Conn

	// local tracks we are sending to others
	tracks []*webrtc.TrackLocalStaticRTP
}

var peers []*Peer = make([]*Peer, 0)

type Msg struct {
	Type      string `json:"type"`
	Sdp       string `json:"sdp,omitempty"`
	Candidate string `json:"new-ice-candidate,omitempty"`
}

type Answer struct {
	Answer webrtc.SessionDescription `json:"answer"`
}

// limiting the udp port range
func getWebRTCAPI() *webrtc.API {
	se := webrtc.SettingEngine{}
	se.SetEphemeralUDPPortRange(50000, 51000)

	mediaEngine := webrtc.MediaEngine{}
	_ = mediaEngine.RegisterDefaultCodecs()

	api := webrtc.NewAPI(webrtc.WithSettingEngine(se), webrtc.WithMediaEngine(&mediaEngine))

	return api
}

func addTrack(other *Peer, track *webrtc.TrackLocalStaticRTP) {
	_, err := other.peerConn.AddTrack(track)
	if err != nil {
		fmt.Println("adding track to peer err: ", err)
		return
	}

	// offer
	{
		offer, err := other.peerConn.CreateOffer(nil)
		if err != nil {
			fmt.Println("offer create err:", err)
			return
		}
		err = other.peerConn.SetLocalDescription(offer)
		if err != nil {
			fmt.Println("set local desc err:", err)
			return
		}

		jsonOffer, _ := json.Marshal(Msg{Type: "offer", Sdp: offer.SDP})
		other.ws.WriteMessage(websocket.TextMessage, jsonOffer)
	}

}

func onTrackHandler(peer *Peer) func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
	return func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		fmt.Println("Got track:", track.Kind())

		// Create a local track to send to the other peer
		var trackType string
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			trackType = "audio"
		}
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			trackType = "video"
		}

		localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, trackType, "sfu")
		if err != nil {
			fmt.Println("new track creation err: ", err)
			return
		}
		fmt.Println("new track", localTrack.ID())
		peer.tracks = append(peer.tracks, localTrack)

		// catch send RTP packets from browser and write to local
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

		for _, other := range peers {
			if other.peerConn == peer.peerConn || other.peerConn == nil {
				continue // don't forward to the same peer
			}
			if other.peerConn.ConnectionState() == webrtc.PeerConnectionStateClosed {
				continue
			}

			// send local track to other peers
			addTrack(other, localTrack)
		}
	}
}

func mediaHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Upgrade error:", err)
		return
	}
	defer ws.Close()

	fmt.Println(blue("ws from " + r.Header.Get("X-Forwarded-For")))

	api := getWebRTCAPI()
	peerConn, err := api.NewPeerConnection(webrtc.Configuration{})
	peer := Peer{peerConn: peerConn, ws: ws}
	peer.tracks = make([]*webrtc.TrackLocalStaticRTP, 0)
	peers = append(peers, &peer)
	defer func() {
		for i, p := range peers {
			if p == &peer {
				peers = append(peers[:i], peers[i+1:]...)
				break
			}
		}
		fmt.Println("removing peer, peers:", peers)
	}()

	// forward old local tracks from rest of the peers now the new peer
	for _, other := range peers {
		if other.peerConn == peer.peerConn {
			continue
		}
		for _, track := range other.tracks {
			addTrack(&peer, track)
		}
	}

	peerConn.OnTrack(onTrackHandler(&peer))

	peerConn.OnICECandidate(func(c *webrtc.ICECandidate) {
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

		switch msg.Type {
		case "offer":
			peerConn.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.Sdp,
			})

			answer, err := peerConn.CreateAnswer(&webrtc.AnswerOptions{})
			if err != nil {
				fmt.Println("create answer error:", err)
				break
			}

			peerConn.SetLocalDescription(answer)

			ws.WriteJSON(answer)

		case "answer":
			peerConn.SetRemoteDescription(
				webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  msg.Sdp,
				})

		case "new-ice-candidate":
			peerConn.AddICECandidate(webrtc.ICECandidateInit{Candidate: msg.Candidate})
		}
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
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", PORT))
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

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

type handler struct{}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

var fire *websocket.Conn = nil
var chro *websocket.Conn = nil

type Peer struct {
	peerConn    *webrtc.PeerConnection
	ws          *websocket.Conn
	tracks      []*webrtc.TrackRemote
	addedTracks map[*webrtc.TrackRemote]bool
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

func addTrack(other *Peer, track *webrtc.TrackRemote) {
	// Create a local track to send to the other peer
	localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, "audio", "sfu")
	if err != nil {
		fmt.Println("new track creation err: ", err)
		return
	}

	sender, err := other.peerConn.AddTrack(localTrack)
	if err != nil {
		fmt.Println("adding track to peer err: ", err)
		return
	}

	other.addedTracks[track] = true

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

func onTrackHandler(peer *Peer) func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
	return func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		fmt.Println("Got track:", track.Kind())
		peer.tracks = append(peer.tracks, track)

		for _, other := range peers {
			if other.peerConn == peer.peerConn && other.peerConn != nil {
				continue // don't forward to the same peer
			}
			if other.peerConn.ConnectionState() == webrtc.PeerConnectionStateClosed {
				continue
			}

			// send new track to other peer
			addTrack(other, track)

			// send old tracks from other peer to the rest
			for _, other2 := range peers {
				if other2.peerConn == other.peerConn {
					continue
				}
				for _, otherTrack := range other2.tracks {
					// skip already added tracks
					if other2.addedTracks[otherTrack] {
						fmt.Println("skiP")
						continue
					}

					fmt.Println("adding older")
					addTrack(other2, otherTrack)
				}
			}
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

	fmt.Println("ws from", r.RemoteAddr)

	api := getWebRTCAPI()
	peerConn, err := api.NewPeerConnection(webrtc.Configuration{})
	peer := Peer{peerConn: peerConn, ws: ws}
	peer.tracks = make([]*webrtc.TrackRemote, 0)
	peer.addedTracks = make(map[*webrtc.TrackRemote]bool, 0)
	peers = append(peers, &peer)

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

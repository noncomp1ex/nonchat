if (location.hostname == "localhost")
  window.DEVENV = true
else
  window.DEVENV = false

let backendURL
if (window.DEVENV)
  backendURL = "http://localhost:8081"
else
  backendURL = "https://crol.bar"


let screenShareButton = document.querySelector('button#shareScreen')
screenShareButton.disabled = true

let openWSButton = document.querySelector('button#openWS')


const shareMicAudio = () => {
  navigator.mediaDevices.getUserMedia(
    {
      'video': false,
      'audio': {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
    }
  ).then(stream => {
    console.log("mic stream", stream)

    const audioElement = document.querySelector('audio#playback');
    audioElement.srcObject = stream;
  })
}


let ws
let peer

const openWS = () => {
  const btn = document.getElementById('openWS');
  btn.classList.remove('spin'); // reset animation if clicked multiple times
  void btn.offsetWidth; // force reflow so animation restarts
  btn.classList.add('spin');

  let url = backendURL.replace("http", "ws") + "/api/media"

  ws = new WebSocket(url)

  ws.onopen = () => {
    window.WSOPENED = true
    openWSButton.classList.add('active');
    openWSButton.textContent = "Connected"
    openWSButton.disabled = true

    peer = new RTCPeerConnection()

    peer.onicecandidate = event => {
      if (event.candidate) {
        ws.send(JSON.stringify({
          type: "new-ice-candidate",
          candidate: event.candidate
        }));
      }
    }

    navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      }
    }).then((stream) => {
      stream.getTracks().forEach(track => peer.addTrack(track, stream))
      return peer.createOffer()
    }).then(async offer => {
      await peer.setLocalDescription(offer);
      ws.send(JSON.stringify(offer));
    })

    peer.addEventListener('connectionstatechange', event => {
      if (peer.connectionState === 'connected') {
        console.log("connected")
        screenShareButton.disabled = false
      }
    });

    peer.ontrack = (event) => {
      console.log("Got ", event.track.kind ," track", event)
      if (event.track.kind == "audio") {
        document.querySelector("audio#remote-audio").srcObject = event.streams[0]
      }
      if (event.track.kind == "video") {
        const videoEl = document.querySelector('video#remote-video')
        console.log("Setting video element:", videoEl)


        videoEl.srcObject = event.streams[0]

        videoEl.play()

        videoEl.onloadedmetadata = () => console.log("Video metadata loaded")
        videoEl.oncanplay = () => console.log("Video can play")
        videoEl.onerror = (e) => console.error("Video error:", e)
      }
    }
  }

  ws.onmessage = async (msg) => {
    console.log("Server says:", msg.data);

    json = JSON.parse(msg.data)

    if (json.type == "answer") {
      const remoteDesc = new RTCSessionDescription(json);
      await peer.setRemoteDescription(remoteDesc);
    }

    if (json.type == "candidate") {
      try {
        const candidate = JSON.parse(json["new-ice-candidate"])
        await peer.addIceCandidate(candidate)
      } catch (error) {
        console.error("Error adding ICE candidate:", error)
      }
    }

    if (json.type == "offer") {
      console.log("offer")
      await peer.setRemoteDescription({ type: "offer", sdp: json.sdp })
      const answer = await peer.createAnswer()
      await peer.setLocalDescription(answer)
      console.log(answer)
      ws.send(JSON.stringify(answer))
    }
  }

  ws.onclose = () => {
    document.querySelector('button#openWS').classList.remove('active');
    document.querySelector('button#openWS').textContent = "Connect"
    screenShareButton.disabled = true
    openWSButton.disabled = false
  }
}

const shareScreen = () => {
  navigator.mediaDevices.getDisplayMedia({
    video: true, // https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#instance_properties_of_video_tracks
    audio: true, // https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#instance_properties_of_audio_tracks
  }).then(stream => {
    stream.getVideoTracks().forEach(track => peer.addTrack(track, stream))

    document.querySelector('video#preview').srcObject = stream
    return peer.createOffer()
  }).then(async offer => {
    await peer.setLocalDescription(offer);
    ws.send(JSON.stringify(offer));
  })
}

const writeWS = () => {
  if (!window.WSOPENED) {
    return
  }

  const text = document.querySelector('input#wstext').value
  console.log(text)

  ws.send(text)
}

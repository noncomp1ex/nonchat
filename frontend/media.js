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
let camShareButton = document.querySelector('button#shareCam')
camShareButton.disabled = true

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
      },
      video: false,
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
        camShareButton.disabled = false
      }
    });

    peer.ontrack = (event) => {
      console.log("Got ", event.track.kind, " track", event)

      switch (event.track.kind) {
        case "audio":
          document.querySelector("audio#remote-audio").srcObject = event.streams[0]
          break;
        case "video":

          switch (event.track.label) {
            case "video": // cam
              const videocamEL = document.querySelector('video#remote-video-cam')
              videocamEL.srcObject = new MediaStream([event.track])
              break;
            case "remote video": // screen share
              const videoscreenEl = document.querySelector('video#remote-video-screen')
              videoscreenEl.srcObject = new MediaStream([event.track])
              break;
          }

          break
      }

    }
  }

  ws.onmessage = async (msg) => {
    json = JSON.parse(msg.data)

    if (json.type == "answer") {
      await peer.setRemoteDescription({ type: "answer", sdp: json.sdp });
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
      await peer.setRemoteDescription({ type: "offer", sdp: json.sdp })
      const answer = await peer.createAnswer()
      await peer.setLocalDescription(answer)
      ws.send(JSON.stringify({ type: "answer", sdp: answer.sdp }))
    }
  }

  ws.onclose = () => {
    document.querySelector('button#openWS').classList.remove('active');
    document.querySelector('button#openWS').textContent = "Connect"
    screenShareButton.disabled = true
    camShareButton.disabled = true
    openWSButton.disabled = false
  }
}

const shareScreen = () => {
  navigator.mediaDevices.getDisplayMedia({
    video: {
      width: { ideal: 1920 },
      height: { ideal: 1080 },
      frameRate: { ideal: 30, max: 30 },
    }, // https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#instance_properties_of_video_tracks
    audio: false, // https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#instance_properties_of_audio_tracks
  }).then(stream => {
    stream.getTracks().forEach(track => peer.addTrack(track, stream))

    const videoPreview = document.querySelector('video#preview')
    videoPreview.srcObject = stream
    videoPreview.muted = true
    return peer.createOffer()
  }).then(async offer => {
    await peer.setLocalDescription(offer);
    ws.send(JSON.stringify(offer));
  })
}

const shareCam = () => {
  navigator.mediaDevices.getUserMedia({
    audio: false,
    video: true,
  }).then((stream) => {
    stream.getTracks().forEach(track => peer.addTrack(track, stream))
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

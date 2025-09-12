if (location.hostname == "localhost")
  window.DEVENV = true
else
  window.DEVENV = false

let backendURL
if (window.DEVENV)
  backendURL = "http://localhost:8081"
else
  backendURL = "http://192.168.1.12:8081"


let input = document.querySelector('input#wstext')
let button = document.querySelector('button#writeWS')

button.disabled = input.value.trim() === "" || !window.WSOPENED;
input.addEventListener("input", () => {
  button.disabled = input.value.trim() === "" || !window.WSOPENED;
});

const shareScreen = () => {
  navigator.mediaDevices.getDisplayMedia({
    video: true, // https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#instance_properties_of_video_tracks
    audio: true, // https://developer.mozilla.org/en-US/docs/Web/API/MediaTrackConstraints#instance_properties_of_audio_tracks
  }).then(stream => {
    console.log("screen stream tracks", stream.getTracks())

    const videoElement = document.querySelector('video#preview');
    videoElement.srcObject = stream;
  })
}

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
  let url = backendURL.replace("http", "ws") + "/api/media"

  ws = new WebSocket(url)

  ws.onopen = () => {
    window.WSOPENED = true
    document.querySelector('button#openWS').disabled = true

    peer = new RTCPeerConnection()

    peer.onicecandidate = event => {
      if (event.candidate) {
        ws.send(JSON.stringify({
          type: "new-ice-candidate",
          candidate: event.candidate
        }));
      }
    }

    navigator.mediaDevices.getUserMedia({ audio: true }).then(stream => {
      stream.getTracks().forEach(track => peer.addTrack(track, stream))

      return peer.createOffer()
    }).then(async offer => {
      await peer.setLocalDescription(offer);
      ws.send(JSON.stringify(offer));
    })


    peer.addEventListener('connectionstatechange', event => {
      if (peer.connectionState === 'connected') {
        console.log("connected")
      }
    });

    peer.ontrack = (event) => {
      console.log("ontrack", event)
      document.getElementById("remote").srcObject = event.streams[0]
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
      await peer.addIceCandidate(JSON.parse(json["new-ice-candidate"]))
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
    document.querySelector('button#openWS').disabled = false
  }
}

const writeWS = () => {
  if (!window.WSOPENED) {
    return
  }

  const text = document.querySelector('input#wstext').value
  console.log(text)

  ws.send(text)
}

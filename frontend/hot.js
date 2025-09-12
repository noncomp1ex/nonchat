if (location.hostname == "localhost")
  (new WebSocket("ws://localhost:9071")).onmessage = (event) => {
    location.reload()
  }

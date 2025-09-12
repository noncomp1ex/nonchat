#!/usr/bin/env python3
import asyncio
import time
import websockets
import subprocess

FRONTEND_DIR = "./frontend"
PORT = 9071
clients = set()

async def handler(websocket):
    clients.add(websocket)
    try:
        await websocket.wait_closed()
    finally:
        clients.remove(websocket)

async def main():
    server = await websockets.serve(handler, "localhost", PORT)

    print(f"🔌 Live reload server running at ws://localhost:{PORT}")

    proc = await asyncio.create_subprocess_exec(
        "inotifywait", "-m", "-r", "-e", "modify,create,delete",
        FRONTEND_DIR, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    )

    print(f"🔄 Watching {FRONTEND_DIR} for changes...")

    async for line in proc.stdout:
        print(f"Change detected: {line.decode().strip()}")
        for client in list(clients):
            try:
                await client.send("reload")
            except:
                pass

    server.wait_closed()

asyncio.run(main())


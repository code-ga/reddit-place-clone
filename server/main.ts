import { config } from "dotenv";
import express from "express";
import { createWriteStream, lstatSync } from "fs";
import http from "http";
import pixel from "image-pixels";
import { PNG } from "pngjs";
import { Server } from "socket.io";

config();
const width = Number(process.env.WIDTH || 1000);
const height = Number(process.env.HEIGHT || 1000);

const app = express();
const server = http.createServer(app);
const io = new Server(server);

let canvas: Uint8Array;

const placeFile = process.env.PLACE_FILE || "place.png";
const placeFileStat = lstatSync(placeFile, { throwIfNoEntry: false });
if (placeFileStat && placeFileStat.isFile()) {
  const lastSavedCanvas = await pixel(placeFile);
  canvas = lastSavedCanvas.data;
} else {
  canvas = Uint8Array.from({ length: width * height * 3 }, () => 255);
}

function saveCanvas() {
  const png = new PNG({ width, height });
  png.data = Buffer.from(canvas.buffer);
  png.pack().pipe(createWriteStream(placeFile));
}

["SIGINT", "SIGTERM", "SIGQUIT"].forEach((signal) =>
  process.on(signal, () => {
    saveCanvas();
    process.exit();
  })
);

setInterval(saveCanvas, 1000 * Number(process.env.SAVE_INTERVAL || 60));

app.get("/place.png", (request, response) => {
  const png = new PNG({ width, height });
  png.data = Buffer.from(canvas);
  response.setHeader("Content-Type", "image/png");
  response.setHeader("Cache-Control", "no-cache");
  png.pack().pipe(response);
});

class InvalidPlace extends Error {}
io.on("connection", (socket) => {
  socket.on("place", (x: number, y: number, color: Buffer) => {
    try {
      if (x < 0 || x >= width || y < 0 || y >= height) throw new InvalidPlace();
      if (color.length !== 3) throw new InvalidPlace();

      for (const c of color) if (c < 0 || c > 255) throw new InvalidPlace();
    } catch (error) {
      if (error instanceof InvalidPlace) return;
    }

    console.log("Received place:", x, y, color);
    io.emit("place", x, y, color);
    const index = (y * width + x) * 3;
    canvas[index] = color[0];
    canvas[index + 1] = color[1];
    canvas[index + 2] = color[2];
  });
});

const port = 80;
server.listen(port, () => {
  console.log(`Server is running on port ${port}`);
});

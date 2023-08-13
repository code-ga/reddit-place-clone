import { io } from "https://cdn.jsdelivr.net/npm/socket.io-client@4.7.1/+esm";

export default class Place {
  #loaded;
  #socket;
  #loadingp;
  #uiwrapper;
  #glWindow;

  constructor(glWindow) {
    this.#loaded = false;
    this.#socket = null;
    this.#loadingp = document.querySelector("#loading-p");
    this.#uiwrapper = document.querySelector("#ui-wrapper");
    this.#glWindow = glWindow;
  }

  initConnection() {
    this.#loadingp.innerHTML = "connecting";

    let host = window.location.hostname;
    let port = window.location.port;
    if (port != "") {
      host += ":" + port;
    }

    let wsProt;
    if (window.location.protocol == "https:") {
      wsProt = "wss:";
    } else {
      wsProt = "ws:";
    }

    this.#connect(wsProt + "//" + host);
    this.#loadingp.innerHTML = "downloading map";

    fetch(window.location.protocol + "//" + host + "/place.png").then(
      async (resp) => {
        if (!resp.ok) {
          console.error("Error downloading map.");
          return null;
        }

        // let buf = await this.#downloadProgress(resp);
        await this.#setImage(await resp.arrayBuffer());

        this.#loaded = true;
        this.#loadingp.innerHTML = "";
        this.#uiwrapper.setAttribute("hide", true);
      }
    );
  }

  async #downloadProgress(resp) {
    let len = resp.headers.get("Content-Length");
    let a = new Uint8Array(len);
    let pos = 0;
    let reader = resp.body.getReader();
    while (true) {
      let { done, value } = await reader.read();
      if (value) {
        a.set(value, pos);
        pos += value.length;
        this.#loadingp.innerHTML =
          "downloading map " + Math.round((pos / len) * 100) + "%";
      }
      if (done) break;
    }
    return a;
  }

  #connect(path) {
    this.#socket = io(path, { transports: ["websocket"] });

    this.#socket.on("connect", () => {
      console.log("Connected to WebSocket.");
    });

    this.#socket.on("place", (x, y, color) => {
      this.#handleSocketSetPixel(x, y, new Uint8Array(color));
    });

    this.#socket.on("disconnect", () => {
      this.#socket = null;
    });

    this.#socket.on("error", () => {
      console.error("Error making WebSocket connection.");
      alert("Failed to connect.");
      this.#socket.close();
      this.#socket = null;
    });
  }

  setPixel(x, y, color) {
    if (this.#socket != null && this.#socket.connected) {
      this.#socket.emit("place", Math.floor(x), Math.floor(y), color);
      this.#glWindow.setPixelColor(x, y, color);
      this.#glWindow.draw();
    } else {
      alert("Disconnected.");
      console.error("Disconnected.");
    }
  }

  #handleSocketSetPixel(x, y, color) {
    if (this.#loaded) {
      this.#glWindow.setPixelColor(x, y, color);
      this.#glWindow.draw();
    }
  }

  async #setImage(data) {
    let img = new Image();
    let blob = new Blob([data], { type: "image/png" });
    let blobUrl = URL.createObjectURL(blob);
    img.src = blobUrl;
    let promise = new Promise((resolve, reject) => {
      img.onload = () => {
        this.#glWindow.setTexture(img);
        this.#glWindow.draw();
        resolve();
      };
      img.onerror = reject;
    });
    await promise;
  }
}

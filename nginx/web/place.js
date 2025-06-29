class Place {
  #loaded;
  #socket;
  #loadingp;
  #uiwrapper;
  #glWindow;

  #dirty = false;

  constructor(glWindow) {
    this.#loaded = false;
    this.#socket = null;
    this.#loadingp = document.querySelector("#loading-p");
    this.#uiwrapper = document.querySelector("#ui-wrapper");
    this.#glWindow = glWindow;

    setInterval(() => {
      if (!this.#loaded) return;
      if (!this.#dirty) return;
      this.#dirty = false;

      this.#glWindow.draw();
    }, 1000 / 100);
  }

  async initConnection() {
    this.#uiwrapper.setAttribute("hide", false);
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

    await this.#connect(wsProt + "//" + host + "/ws");
    this.#loadingp.innerHTML = "downloading map";

    fetch(window.location.protocol + "//" + host + "/place.png").then(async (resp) => {
      if (!resp.ok) {
        console.error("Error downloading map.");
        return null;
      }

      let buf = new Uint8Array(await resp.arrayBuffer());
      // let buf = await this.#downloadProgress(resp);
      await this.#setImage(buf);

      this.#loaded = true;
      this.#loadingp.innerHTML = "";
      this.#uiwrapper.setAttribute("hide", true);
    });
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
        this.#loadingp.innerHTML = "downloading map " + Math.round((pos / len) * 100) + "%";
      }
      if (done) break;
    }
    return a;
  }

  async #connect(path) {
    this.#socket = new WebSocket(path);

    const waitForConnection = new Promise((resolve) =>
      this.#socket.addEventListener("open", () => {
        console.log("Connected to server.");
        resolve();
      })
    );

    const socketMessage = async (event) => {
      let b = await event.data.arrayBuffer();
      this.#handleSocketSetPixel(b);
    };

    const socketClose = (event) => {
      this.#socket = null;
      console.log("Disconnected from server.");

      // try reconnect
      this.initConnection();
    };

    const socketError = (event) => {
      console.error("Error making WebSocket connection.");
      this.#socket.close();
    };

    this.#socket.addEventListener("message", socketMessage);
    this.#socket.addEventListener("close", socketClose);
    this.#socket.addEventListener("error", socketError);
  }

  setPixel(x, y, color) {
    if (this.#socket != null && this.#socket.readyState == 1) {
      let b = new Uint8Array(11);
      this.#putUint32(b.buffer, 0, x);
      this.#putUint32(b.buffer, 4, y);
      for (let i = 0; i < 3; i++) {
        b[8 + i] = color[i];
      }
      this.#socket.send(b);
      this.#glWindow.setPixelColor(x, y, color);
      this.#glWindow.draw();
    } else {
      alert("Disconnected.");
      console.error("Disconnected.");
    }
  }

  /**
   *
   * @param {ArrayBuffer} b
   */
  #handleSocketSetPixel(b) {
    let bytes = b;
    let view = new DataView(b);

    let offset = 0;

    if (!this.#loaded) return;

    while (offset < bytes.byteLength) {
      const x = view.getUint32(offset, false);
      const y = view.getUint32(offset + 4, false);
      const color = new Uint8Array(bytes.slice(offset + 8, offset + 11));
      this.#glWindow.setPixelColor(x, y, color);
      offset += 11;
    }

    this.#dirty = true;
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

  #putUint32(b, offset, n) {
    let view = new DataView(b);
    view.setUint32(offset, n, false);
  }

  #getUint32(b, offset) {
    let view = new DataView(b);
    return view.getUint32(offset, false);
  }
}

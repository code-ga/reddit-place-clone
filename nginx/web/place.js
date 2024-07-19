class Place {
  /**
   * @type {boolean}
   */
  loaded = false;

  /**
   * @type {WebSocket | undefined}
   */
  ws;

  /**
   * @type {HTMLElement | null}
   */
  loadingText = null;

  /**
   * @type {HTMLElement | null}
   */
  ui = null;

  gl;

  constructor(gl) {
    this.loadingText = document.querySelector("#loading-text");
    this.ui = document.querySelector("#ui");

    this.gl = gl;
  }

  async initConnection() {
    this.ui.setAttribute("hide", false);
    this.loadingText.innerHTML = "connecting";

    const host = window.location.hostname + (window.location.port != "" ? ":" + window.location.port : "");

    this.loadingText.innerHTML = "downloading map";
    this.connect((window.location.protocol == "https:" ? "wss:" : "ws:") + "//" + host + "/ws");

    const response = await fetch(window.location.protocol + "//" + host + "/place.png");
    if (!response.ok) {
      console.error("Error downloading map.");
      return null;
    }

    await this.setImage(new Uint8Array(await response.arrayBuffer()));

    this.loaded = true;
    this.loadingText.innerHTML = "";
    this.ui.setAttribute("hide", true);
  }

  connect(path) {
    this.ws = new WebSocket(path);

    this.ws.addEventListener("message", async (event) => {
      this.receivePixel(await event.data.arrayBuffer());
    });

    this.ws.addEventListener("close", (event) => {
      this.ws = undefined;
      console.log("Disconnected from server.");

      // try reconnect
      this.initConnection();
    });

    this.ws.addEventListener("error", (event) => {
      console.error("Error making WebSocket connection.");
      this.ws.close();
    });
  }

  sendPixel(x, y, color) {
    if (this.ws == null || this.ws.readyState != 1) {
      alert("Disconnected.");
      console.error("Disconnected.");
      return;
    }

    const message = new Uint8Array(11);

    const view = new DataView(message);

    view.setUint32(0, x, false);
    view.setUint32(4, y, false);
    message[8] = color[0]; // r
    message[9] = color[1]; // g
    message[10] = color[2]; // b

    this.ws.send(message);

    this.gl.setPixelColor(x, y, color);
    this.gl.draw();
  }

  /**
   *
   * @param {ArrayBuffer} bytes
   */
  receivePixel(bytes) {
    const view = new DataView(bytes);

    let offset = 0;

    if (!this.loaded) return;

    while (offset < bytes.byteLength) {
      const x = view.getUint32(offset, false);
      const y = view.getUint32(offset + 4, false);
      const color = new Uint8Array(bytes.slice(offset + 8, offset + 11));
      this.gl.setPixelColor(x, y, color);
      offset += 11;
    }

    this.gl.draw();
  }

  async setImage(data) {
    const img = new Image();
    img.src = URL.createObjectURL(new Blob([data], { type: "image/png" }));
    await new Promise((resolve, reject) => {
      img.onload = () => {
        this.gl.setTexture(img);
        this.gl.draw();
        resolve();
      };
      img.onerror = reject;
    });
  }
}

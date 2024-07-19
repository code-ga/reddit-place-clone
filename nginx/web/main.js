function main() {
  const cvs = document.querySelector("#viewport-canvas");
  const glWindow = new GLWindow(cvs);

  if (!glWindow.ok()) return;

  const place = new Place(glWindow);
  place.initConnection();

  GUI(cvs, glWindow, place);
}

/**
 * @typedef {{
 *  x: number,
 *  y: number
 * }} Position
 */

/**
 *
 * @param {HTMLElement} cvs
 * @param {any} glWindow
 * @param {Place} place
 */
function GUI(cvs, glWindow, place) {
  let color = new Uint8Array([0, 0, 0]);
  let dragdown = false;
  let touchID = 0;
  let touchScaling = false;
  let lastMovePos = { x: 0, y: 0 };
  let lastScalingDist = 0;
  let touchstartTime = 0;

  const colorField = document.querySelector("#color-field");
  const colorSwatch = document.querySelector("#color-swatch");

  // ***************************************************
  // ***************************************************
  // Event Listeners
  //
  document.addEventListener("keydown", (ev) => {
    switch (ev.keyCode) {
      case 189:
      case 173:
        ev.preventDefault();
        zoom(1.2, false);
        break;
      case 187:
      case 61:
        ev.preventDefault();
        zoom(1.2);
        break;
    }
  });

  window.addEventListener("wheel", (event) => zoom(1.05, event.deltaY <= 0));

  document.querySelector("#zoom-in").addEventListener("click", () => zoom(1.2));
  document.querySelector("#zoom-out").addEventListener("click", () => zoom(1.2, false));

  window.addEventListener("resize", () => {
    glWindow.updateViewScale();
    glWindow.draw();
  });

  cvs.addEventListener("mousedown", (event) => {
    const xy = { x: event.clientX, y: event.clientY };
    switch (event.button) {
      case 0:
        dragdown = true;
        lastMovePos = xy;
        console.log(glWindow.click(xy));
        break;
      case 1:
        pickColor(xy);
        break;
      case 2:
        if (event.ctrlKey) pickColor(xy);
        else drawPixel(xy, color);
    }
  });

  document.addEventListener("mouseup", () => {
    dragdown = false;
    document.body.style.cursor = "auto";
  });

  document.addEventListener("mousemove", (event) => {
    const movePos = { x: event.clientX, y: event.clientY };
    if (dragdown) {
      glWindow.move(movePos.x - lastMovePos.x, movePos.y - lastMovePos.y);
      glWindow.draw();
      document.body.style.cursor = "grab";
    }
    lastMovePos = movePos;
  });

  cvs.addEventListener("touchstart", (event) => {
    let thisTouch = touchID;
    touchstartTime = new Date().getTime();
    lastMovePos = { x: event.touches[0].clientX, y: event.touches[0].clientY };
    if (event.touches.length === 2) {
      touchScaling = true;
      lastScalingDist = null;
    }

    setTimeout(() => {
      if (thisTouch != touchID) return;
      pickColor(lastMovePos);
      navigator.vibrate(200);
    }, 350);
  });

  document.addEventListener("touchend", (event) => {
    touchID++;
    if (new Date().getTime() - touchstartTime < 100 && drawPixel(lastMovePos, color)) navigator.vibrate(10);
    if (event.touches.length === 0) touchScaling = false;
  });

  document.addEventListener("touchmove", (event) => {
    touchID++;
    if (touchScaling) {
      const dist = Math.hypot(event.touches[0].pageX - event.touches[1].pageX, event.touches[0].pageY - event.touches[1].pageY);
      if (lastScalingDist != null) {
        const delta = lastScalingDist - dist;
        zoom(1 + Math.abs(delta) * 0.003, delta < 0);
      }
      lastScalingDist = dist;
    } else {
      const movePos = { x: event.touches[0].clientX, y: event.touches[0].clientY };
      glWindow.move(movePos.x - lastMovePos.x, movePos.y - lastMovePos.y);
      glWindow.draw();
      lastMovePos = movePos;
    }
  });

  cvs.addEventListener("contextmenu", () => false);

  colorField.addEventListener("change", () => {
    let hex = colorField.value.replace(/[^A-Fa-f0-9]/g, "").toUpperCase();
    hex = hex.substring(0, 6);
    while (hex.length < 6) hex += "0";

    color[0] = parseInt(hex.substring(0, 2), 16);
    color[1] = parseInt(hex.substring(2, 4), 16);
    color[2] = parseInt(hex.substring(4, 6), 16);
    hex = "#" + hex;
    colorField.value = hex;
    colorSwatch.style.backgroundColor = hex;
  });

  // ***************************************************
  // ***************************************************
  // Helper Functions
  //

  /**
   *
   * @param {Position} pos
   */
  function pickColor(pos) {
    color = glWindow.getColor(glWindow.click(pos));
    let hex = "#";
    for (let i = 0; i < color.length; i++) {
      let d = color[i].toString(16);
      if (d.length == 1) d = "0" + d;
      hex += d;
    }
    colorField.value = hex.toUpperCase();
    colorSwatch.style.backgroundColor = hex;
  }

  /**
   *
   * @param {Position} pos
   * @param {Uint8Array} color
   * @returns {boolean}
   */
  function drawPixel(pos, color) {
    pos = glWindow.click(pos);
    if (!pos) return false;

    const oldColor = glWindow.getColor(pos);

    for (let i = 0; i < oldColor.length; i++) {
      if (oldColor[i] == color[i]) continue;

      place.setPixel(pos.x, pos.y, color);
      return true;
    }

    return false;
  }

  /**
   *
   * @param {number} factor
   * @param {boolean} zoomIn
   */
  function zoom(factor, zoomIn = true) {
    let zoom = glWindow.getZoom();
    if (zoomIn) zoom *= factor;
    else zoom /= factor;

    glWindow.setZoom(zoom);
    glWindow.draw();
  }
}

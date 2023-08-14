package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	width        = 1280
	height       = 720
	saveInterval = 60 // seconds
	pingInterval = 30 // seconds
	canvasFile   = "data/place.png"
	connections  = 1000
)

var (
	canvas      = make([]byte, 4*width*height)
	canvasMutex = &sync.Mutex{}
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	clients = make([]*websocket.Conn, 1000)
)

func initCanvas() {
	if _, err := os.Stat(canvasFile); err == nil {
		file, err := os.Open(canvasFile)
		if err != nil {
			fmt.Println("Error opening canvas file:", err)
			return
		}
		defer file.Close()

		img, _, err := image.Decode(file)
		if err != nil {
			fmt.Println("Error decoding image:", err)
			return
		}

		canvasMutex.Lock()
		defer canvasMutex.Unlock()

		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				index := 4 * ((y * width) + x)
				canvas[index] = uint8(r >> 8)
				canvas[index+1] = uint8(g >> 8)
				canvas[index+2] = uint8(b >> 8)
				canvas[index+3] = uint8(255)
			}
		}
	}
}

func saveCanvas() {
	canvasMutex.Lock()
	defer canvasMutex.Unlock()

	file, err := os.Create(canvasFile)
	if err != nil {
		fmt.Println("Error creating canvas file:", err)
		return
	}
	defer file.Close()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := 4 * ((y * width) + x)
			r := uint32(canvas[index])
			g := uint32(canvas[index+1])
			b := uint32(canvas[index+2])
			a := uint32(255)
			img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)})
		}
	}

	err = png.Encode(file, img)
	if err != nil {
		fmt.Println("Error encoding canvas to PNG:", err)
	}
}

func placeHandler(w http.ResponseWriter, r *http.Request) {
	canvasMutex.Lock()
	defer canvasMutex.Unlock()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := 4 * ((y * width) + x)
			r := uint32(canvas[index])
			g := uint32(canvas[index+1])
			b := uint32(canvas[index+2])
			a := uint32(255)
			img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)})
		}
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=0")
	png.Encode(w, img)
}

func placeSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection:", err)
		return
	}

	clients = append(clients, conn)

	go func() {
		pingTicker := time.NewTicker(pingInterval * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case <-pingTicker.C:
				err := conn.WriteMessage(websocket.PingMessage, []byte{})
				if err != nil {
					fmt.Println("Error sending ping:", err)
					conn.Close()
					return
				}
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				fmt.Println("WebSocket connection closed:", err)
			} else {
				fmt.Println("Error reading message:", err)
			}
			conn.Close()
			return
		}

		if len(message) != 11 {
			conn.Close()
		}

		x := binary.BigEndian.Uint32(message[0:4])
		y := binary.BigEndian.Uint32(message[4:8])

		if x >= width || y >= height {
			conn.Close()
			return
		}

		color := message[8:11]
		r := uint8(color[0])
		g := uint8(color[1])
		b := uint8(color[2])

		canvasMutex.Lock()

		index := 4 * ((y * width) + x)

		canvas[index] = r
		canvas[index+1] = g
		canvas[index+2] = b

		canvasMutex.Unlock()

		for client := range clients {
			err = clients[client].WriteMessage(websocket.BinaryMessage, message)
			if err != nil {
				fmt.Println("Error writing message:", err)
				clients[client].Close()
				clients[client] = nil
				return
			}
		}

	}
}

func main() {
	initCanvas()

	go func() {
		for {
			time.Sleep(saveInterval * time.Second)
			saveCanvas()
		}
	}()

	http.HandleFunc("/place.png", placeHandler)
	http.HandleFunc("/ws", placeSocketHandler)

	port := 80
	fmt.Printf("Server is running on port %d\n", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}

package main

import (
	"encoding/binary"
	"flag"
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

var (
	address      = flag.String("address", ":80", "Address to listen on")
	width        = flag.Int("width", 1280, "Width of the canvas")
	height       = flag.Int("height", 720, "Height of the canvas")
	saveInterval = flag.Int("save-interval", 60, "Interval to save the canvas (in seconds)")
	pingInterval = flag.Int("ping-interval", 30, "Interval to ping clients (in seconds)")
	canvasFile   = flag.String("save", "place.png", "File to save the canvas to")
	connections  = flag.Int("connections", 5000, "Maximum number of connections")
)

var (
	canvas      = make([]byte, 4*(*width)*(*height))
	canvasMutex = &sync.Mutex{}
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	clients = make([]*websocket.Conn, *connections)
)

func placePixel(x, y int, r, g, b uint8) {
	canvasMutex.Lock()
	defer canvasMutex.Unlock()

	index := 4 * ((y * *width) + x)
	canvas[index] = r
	canvas[index+1] = g
	canvas[index+2] = b
	canvas[index+3] = 255
}

func broadcast(message []byte) {
	for _, client := range clients {
		if client == nil {
			continue
		}

		if err := client.WriteMessage(websocket.BinaryMessage, message); err != nil {
			client.Close()
			continue
		}
	}
}

func initCanvas() {
	if _, err := os.Stat(*canvasFile); err == nil {
		file, err := os.Open(*canvasFile)
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
				index := 4 * ((y * *width) + x)
				canvas[index] = uint8(r >> 8)
				canvas[index+1] = uint8(g >> 8)
				canvas[index+2] = uint8(b >> 8)
				canvas[index+3] = uint8(255)
			}
		}
	} else {
		fmt.Println("Making new canvas...")

		canvasMutex.Lock()

		for y := 0; y < *height; y++ {
			for x := 0; x < *width; x++ {
				index := 4 * ((y * *width) + x)
				canvas[index] = uint8(255)
				canvas[index+1] = uint8(255)
				canvas[index+2] = uint8(255)
				canvas[index+3] = uint8(255)
			}
		}

		canvasMutex.Unlock()

		saveCanvas()
	}
}

func copyCanvasToImage(img *image.RGBA) {
	for y := 0; y < *height; y++ {
		for x := 0; x < *width; x++ {
			index := 4 * ((y * *width) + x)
			r := uint8(canvas[index])
			g := uint8(canvas[index+1])
			b := uint8(canvas[index+2])
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
}

func saveCanvas() {
	canvasMutex.Lock()
	defer canvasMutex.Unlock()

	if _, err := os.Stat(*canvasFile); err != nil {
		file, err := os.Create(*canvasFile)
		if err != nil {
			fmt.Println("Error creating canvas file:", err)
			return
		}
		defer file.Close()
	}

	file, err := os.OpenFile(*canvasFile, os.O_WRONLY, 0600)
	if err != nil {
		fmt.Println("Error opening canvas file:", err)
		return
	}
	defer file.Close()

	img := image.NewRGBA(image.Rect(0, 0, *width, *height))
	copyCanvasToImage(img)

	if err = png.Encode(file, img); err != nil {
		fmt.Println("Error encoding canvas to PNG:", err)
	}
}

func placepng(w http.ResponseWriter, r *http.Request) {
	canvasMutex.Lock()
	defer canvasMutex.Unlock()

	img := image.NewRGBA(image.Rect(0, 0, *width, *height))
	copyCanvasToImage(img)

	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, img)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection:", err)
		return
	}

	conn.SetCloseHandler(
		func(code int, text string) error {
			for index, c := range clients {
				if c == conn {
					clients[index] = nil
					break
				}
			}
			return nil
		},
	)

	clients = append(clients, conn)

	pingTicker := time.NewTicker(time.Duration(*pingInterval) * time.Second)

	go func() {
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
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) || websocket.IsCloseError(err, websocket.CloseGoingAway) {
				fmt.Println("WebSocket connection closed:", err)
			} else {
				fmt.Println("Error reading message:", err)
			}
			conn.Close()
			return
		}

		if len(message) != 11 {
			conn.Close()
			return
		}

		x := int(binary.BigEndian.Uint32(message[0:4]))
		y := int(binary.BigEndian.Uint32(message[4:8]))

		if x >= *width || y >= *height {
			conn.Close()
			return
		}

		r := message[8]
		g := message[9]
		b := message[10]

		index := 4 * ((y * *width) + x)

		if canvas[index] == r && canvas[index+1] == g && canvas[index+2] == b {
			continue
		}

		placePixel(x, y, r, g, b)

		broadcast(message)
	}
}

func main() {

	flag.Parse()

	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true // too lazy to implement proper origin checking
	}
	initCanvas()

	go func() {
		for {
			time.Sleep(time.Duration(*saveInterval) * time.Second)
			saveCanvas()
		}
	}()

	http.HandleFunc("/place.png", placepng)
	http.HandleFunc("/ws", wsHandler)

	fmt.Printf("Server is running on %s\n", *address)
	if err := http.ListenAndServe(*address, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

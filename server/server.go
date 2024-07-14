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
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	address      = flag.String("address", ":80", "Address to listen on")
	width        = flag.Int("width", 1000, "Width of the canvas")
	height       = flag.Int("height", 1000, "Height of the canvas")
	saveInterval = flag.Int("save-interval", 120, "Interval to save the canvas (in seconds)")
	pingInterval = flag.Int("ping-interval", 30, "Interval to ping clients (in seconds)")
	canvasFile   = flag.String("save-location", "place.png", "File to save the canvas to")
	connections  = flag.Int("connections", 20000, "Maximum number of connections")
)

type Canvas struct {
	Width  int
	Height int
	Data   []byte

	// Mutex *sync.Mutex
}

var (
	canvas   Canvas
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	clients = make([]*websocket.Conn, *connections)
)

func (canvas *Canvas) Clear() {
	// canvas.Mutex.Lock()
	// defer canvas.Mutex.Unlock()

	for i := 0; i < 3*canvas.Width*canvas.Height; i++ {
		canvas.Data[i] = uint8(255)
	}
}

func (canvas *Canvas) GetIndex(x, y int) int {
	return 3 * ((y * canvas.Width) + x)
}

func (canvas *Canvas) PlacePixel(x, y int, r, g, b uint8) {
	// canvas.Mutex.Lock()
	// defer canvas.Mutex.Unlock()

	index := canvas.GetIndex(x, y)
	canvas.Data[index] = r
	canvas.Data[index+1] = g
	canvas.Data[index+2] = b
}

func (canvas *Canvas) GetPixel(x, y int) (uint8, uint8, uint8) {
	// canvas.Mutex.Lock()
	// defer canvas.Mutex.Unlock()

	index := canvas.GetIndex(x, y)
	return canvas.Data[index], canvas.Data[index+1], canvas.Data[index+2]
}

func (canvas *Canvas) FromImage(img *image.Image) {
	for y := 0; y < (*img).Bounds().Dy(); y++ {
		for x := 0; x < (*img).Bounds().Dx(); x++ {
			r, g, b, _ := (*img).At(x, y).RGBA()

			canvas.PlacePixel(x, y, uint8(r>>8), uint8(g>>8), uint8(b>>8))
		}
	}

}

func (canvas *Canvas) ToImage(img *image.RGBA) {
	for y := 0; y < canvas.Height; y++ {
		for x := 0; x < canvas.Width; x++ {
			r, g, b := canvas.GetPixel(x, y)
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
}

func (canvas *Canvas) ToFile(filename string) error {
	// canvas.Mutex.Lock()
	// defer canvas.Mutex.Unlock()

	var file *os.File
	defer file.Close()

	if _, err := os.Stat(filename); err != nil {
		file, err = os.Create(filename)
		if err != nil {
			return fmt.Errorf("error creating canvas file: %v", err)
		}
		defer file.Close()
	}

	file, err := os.OpenFile(filename, os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("error opening canvas file: %v", err)
	}
	defer file.Close()

	img := image.NewRGBA(image.Rect(0, 0, *width, *height))
	canvas.ToImage(img)

	if err = png.Encode(file, img); err != nil {
		return fmt.Errorf("error encoding canvas to PNG: %v", err)
	}

	return nil
}

var writeMutex = &sync.Mutex{}

func broadcast(message []byte) {
	writeMutex.Lock()
	defer writeMutex.Unlock()

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

func placepng(w http.ResponseWriter, r *http.Request) {
	// canvas.Mutex.Lock()
	// defer canvas.Mutex.Unlock()

	img := image.NewRGBA(image.Rect(0, 0, *width, *height))
	canvas.ToImage(img)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

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

		cr, cg, cb := canvas.GetPixel(x, y)
		if cr == r && cg == g && cb == b {
			continue
		}

		canvas.PlacePixel(x, y, r, g, b)

		broadcast(message)
	}
}

func initCanvas() error {
	if _, err := os.Stat(*canvasFile); err != nil {
		fmt.Println("Making new canvas...")

		// canvas.Mutex.Lock()

		canvas.Clear()

		// canvas.Mutex.Unlock()

		if err := canvas.ToFile(*canvasFile); err != nil {
			return fmt.Errorf("error saving canvas: %v", err)
		}

		return nil
	}

	file, err := os.Open(*canvasFile)
	if err != nil {
		return fmt.Errorf("error opening canvas file: %v", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("error decoding image: %v", err)
	}

	// canvas.Mutex.Lock()

	canvas.FromImage(&img)

	// defer canvas.Mutex.Unlock()

	fmt.Println("Loaded canvas from file.")

	return nil

}

func main() {
	flag.Parse()

	canvas = Canvas{Width: *width, Height: *height, Data: make([]byte, 3*(*width)*(*height))}

	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true // too lazy to implement proper origin checking
	}

	if err := initCanvas(); err != nil {
		fmt.Println("Error initializing canvas:", err)
		return
	}

	go func() {
		for {
			time.Sleep(time.Duration(*saveInterval) * time.Second)
			if err := canvas.ToFile(*canvasFile); err != nil {
				fmt.Println("Error saving canvas:", err)
			}

			exec.Command("cp", *canvasFile, fmt.Sprintf("%s-%s", *canvasFile, strconv.FormatInt(time.Now().Unix(), 10))).Start()
		}
	}()

	http.HandleFunc("/place.png", placepng)
	http.HandleFunc("/ws", wsHandler)

	fmt.Printf("Server is running on %s\n", *address)
	if err := http.ListenAndServe(*address, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

var (
	address = flag.String("address", ":80", "Address to listen on")

	width  = flag.Int("width", 1000, "Width of the canvas")
	height = flag.Int("height", 1000, "Height of the canvas")

	saveInterval = flag.Int("save-interval", 120, "Interval to save the canvas (in seconds)")
	canvasFile   = flag.String("save-location", "place.png", "File to save the canvas to")

	connections      = flag.Int("connections", 500000, "Maximum number of distinct IP connections")
	connectionsPerIP = flag.Int("connections-per-ip", 3, "Maximum number of connections per IP")

	pingInterval = flag.Int("ping-interval", 30, "Interval to ping clients (in seconds)")
	strikesLimit = flag.Int("strikes-limit", 3, "Number of strikes before disconnecting client")
)

var (
	canvas   Canvas
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	clients      = make(map[string]*[]*Client) // map of IP to Client
	clientsMutex = &sync.Mutex{}
)

type Client struct {
	Conn *websocket.Conn

	LastPixelTimestamp   int64
	LastMessageTimestamp int64
	LastPingTimestamp    int64

	Strikes uint8

	Mutex *sync.Mutex
}

type Canvas struct {
	Width  int
	Height int
	Data   []byte

	// Mutex *sync.Mutex
}

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
	for y := 0; y < canvas.Height; y++ {
		for x := 0; x < canvas.Width; x++ {
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

func broadcast() {
	changesMutex.Lock()

	if len(changes) == 0 {
		changesMutex.Unlock()
		return
	}

	clientsMutex.Lock()
	for _, realClients := range clients {
		for _, client := range *realClients {
			if client == nil {
				continue
			}

			go func(client *Client) {
				client.Mutex.Lock()
				defer client.Mutex.Unlock()
				if err := client.Conn.WriteMessage(websocket.BinaryMessage, changes); err != nil {
					client.Conn.Close()
				}
			}(client)
		}
	}
	clientsMutex.Unlock()

	changes = make([]byte, 0)

	changesMutex.Unlock()

}

func eliminateSleepers() {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for ip, realClients := range clients {
		for _, client := range *realClients {
			if client == nil {
				delete(clients, ip)
				continue
			}

			if time.Now().Unix()-client.LastMessageTimestamp > int64(math.Round(float64(*pingInterval)/5)) {
				client.Strikes++
			} else {
				client.Strikes = 0
			}

			if client.Strikes >= uint8(*strikesLimit) {
				client.Conn.Close()
				delete(clients, ip)
			}

		}
	}

}

func pingAll() {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for _, realClients := range clients {
		for _, client := range *realClients {
			if client == nil {
				continue
			}

			go func(client *Client) {
				client.Mutex.Lock()
				defer client.Mutex.Unlock()
				if err := client.Conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					client.Conn.Close()
				}
				client.LastPingTimestamp = time.Now().Unix()
			}(client)
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

// var changes = make(chan []byte, 10000000) // 11 bytes per message, 1000000 messages ~ 11MB
// big mistakes were made

var changes = make([]byte, 0)
var changesMutex = &sync.Mutex{}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	ip := r.Header.Get("CF-Connecting-IPv6")

	if ip == "" {
		ip = r.Header.Get("CF-Connecting-IP")
	}

	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}

	if ip == "" {
		ip = r.RemoteAddr
	}

	// fmt.Println("New connection from:", ip)

	currentClients := clients[ip]

	if len(*currentClients) >= *connectionsPerIP {
		// too much connections
		// fmt.Println("Client already connected, closing connection...")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	if len(clients) >= *connections {
		// too much connections
		// fmt.Println("Too many connections, closing connection...")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection:", err)
		return
	}

	client := &Client{
		Conn: conn,

		LastPixelTimestamp:   0,
		LastMessageTimestamp: time.Now().Unix(),
		LastPingTimestamp:    time.Now().Unix(),

		Mutex: &sync.Mutex{},
	}

	realClients := append(*currentClients, client)

	// lock cho chắc
	clientsMutex.Lock()
	clients[ip] = &realClients
	clientsMutex.Unlock()

	conn.SetCloseHandler(
		func(code int, text string) error {
			for i, c := range realClients {
				if c == client {
					realClients[i] = nil
					break
				}
			}

			return nil
		},
	)

	// pingTicker := time.NewTicker(time.Duration(*pingInterval) * time.Second)

	// go func() {
	// 	defer pingTicker.Stop()
	// 	for {
	// 		select {
	// 		case <-pingTicker.C:
	// 			err := conn.WriteMessage(websocket.PingMessage, []byte{})
	// 			if err != nil {
	// 				fmt.Println("Error sending ping:", err)
	// 				conn.Close()
	// 				return
	// 			}
	// 		}
	// 	}
	// }()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// if websocket.IsCloseError(err, websocket.CloseNormalClosure) || websocket.IsCloseError(err, websocket.CloseGoingAway) {
			// 	fmt.Println("WebSocket connection closed:", err)
			// } else {
			// 	fmt.Println("Error reading message:", err)
			// }
			conn.Close()
			return
		}

		client.LastMessageTimestamp = time.Now().Unix()

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

		changesMutex.Lock()
		changes = append(changes, message...)
		changesMutex.Unlock()
	}
}

func compactClientList() {
	newClients := make(map[string]*[]*Client)

	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for ip, realClients := range clients {
		var newRealClients []*Client

		for _, client := range *realClients {
			if client != nil {
				newRealClients = append(newRealClients, client)
			}
		}

		if len(newRealClients) > 0 {
			newClients[ip] = &newRealClients
		}
	}

	clients = newClients
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

func StatsHandle(w http.ResponseWriter, r *http.Request) {
	// response number of connections
	// chỗ này nó sẽ tính lun cả những client mà nó nil lun D
	// ko có nil
	w.Write([]byte(strconv.Itoa(len(clients))))
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
			fmt.Println("Number of connections: ", len(clients))
			fmt.Println("Number of changes: ", len(changes))
			time.Sleep(1 * time.Second)

			if len(clients) > *connections-50 {
				// kms
				fmt.Println("Too many connections, shutting down...")
				fmt.Println("Save first tho...")
				if err := canvas.ToFile(*canvasFile); err != nil {
					fmt.Println("Error saving canvas:", err)
				}

				os.Exit(0)
			}
		}
	}()

	go func() {
		for {
			broadcast()
			time.Sleep(25 * time.Millisecond)
		}
	}()

	go func() {
		for {
			time.Sleep(time.Duration(*saveInterval) * time.Second)
			if err := canvas.ToFile(*canvasFile); err != nil {
				fmt.Println("Error saving canvas:", err)
				continue
			}

			exec.Command("cp", *canvasFile, fmt.Sprintf("%s-%s", *canvasFile, strconv.FormatInt(time.Now().Unix(), 10))).Start()
		}
	}()

	go func() {
		for {
			time.Sleep(time.Duration(*pingInterval) * time.Second)
			pingAll()
			compactClientList()
		}
	}()

	go func() {
		for {
			time.Sleep(time.Duration(math.Round(float64(*pingInterval)/5)) * time.Second)
			eliminateSleepers()
		}
	}()

	http.HandleFunc("/place.png", placepng)
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/stats", StatsHandle)

	http.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if err := canvas.ToFile(*canvasFile); err != nil {
			fmt.Println("Error saving canvas:", err)
			return
		}

		exec.Command("cp", *canvasFile, fmt.Sprintf("%s-%s", *canvasFile, strconv.FormatInt(time.Now().Unix(), 10))).Start()

	})

	http.HandleFunc("/safe-restart", func(w http.ResponseWriter, r *http.Request) {
		if err := canvas.ToFile(*canvasFile); err != nil {
			fmt.Println("Error saving canvas:", err)
			return
		}

		exec.Command("cp", *canvasFile, fmt.Sprintf("%s-%s", *canvasFile, strconv.FormatInt(time.Now().Unix(), 10))).Start()

		fmt.Println("Restarting server...")
		os.Exit(0)
	})

	signals := make(chan os.Signal, 1)
	go func() {
		// handle os signals

		sig := <-signals

		if sig == syscall.SIGTERM || sig == syscall.SIGINT {
			fmt.Println("Saving canvas...")
			if err := canvas.ToFile(*canvasFile); err != nil {
				fmt.Println("Error saving canvas:", err)
			}

			os.Exit(0)
		}

	}()

	fmt.Printf("Server is running on %s\n", *address)
	if err := http.ListenAndServe(*address, nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

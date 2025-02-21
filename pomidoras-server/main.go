package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

type State string

const (
	StateCountdown State = "countdown"
	StateIdle      State = "idle"
	SocketPath           = "/tmp/pomidoras.sock" // Use a Unix domain socket
)

type Timer struct {
	duration        time.Duration
	initialDuration time.Duration
	state           State
	ticker          *time.Ticker
	mu              sync.RWMutex
	terminalWidth   int //Added for client
}

type TimerStatus struct {
	State    State         `json:"state"`
	Duration time.Duration `json:"duration"`
}

// Request types for client-server communication
type RequestType string

const (
	RequestTypeStatus     RequestType = "status"
	RequestTypeAddSeconds RequestType = "add_seconds"
	RequestTypeReset      RequestType = "reset" // Added reset request
)

type Request struct {
	Type    RequestType `json:"type"`
	Payload string      `json:"payload,omitempty"` // Use string for flexibility
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Status  TimerStatus `json:"status,omitempty"`
}

func NewTimer(initialDuration time.Duration) *Timer {
	state := StateIdle
	if initialDuration > 0 {
		state = StateCountdown
	}

	width, _, err := term.GetSize(int(os.Stdout.Fd())) // Get terminal size, added for client
	if err != nil {
		width = 80 // Default width if we can't get the size
	}

	return &Timer{
		duration:        initialDuration,
		initialDuration: initialDuration,
		state:           state,
		terminalWidth:   width, //Added for client
	}
}

func (t *Timer) Start() {
	if t.duration > 0 {
		t.mu.Lock()
		t.state = StateCountdown
		t.ticker = time.NewTicker(1 * time.Second)
		t.mu.Unlock()
		go t.run()
	} else {
		t.mu.Lock()
		t.state = StateIdle
		t.mu.Unlock()
	}
}

func (t *Timer) run() {
	for range t.ticker.C {
		t.mu.Lock()
		t.duration -= time.Second
		if t.duration <= 0 {
			t.state = StateIdle
			t.ticker.Stop()
			t.duration = 0
			t.sendNotification("Pomidoras", "Time's up!") // Send notification
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()
	}
}

func (t *Timer) AddSeconds(seconds int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.duration += time.Duration(seconds) * time.Second
	if t.state == StateIdle && t.duration > 0 {
		t.state = StateCountdown
		t.ticker = time.NewTicker(1 * time.Second)
		go t.run()
	}
}

func (t *Timer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.duration = t.initialDuration
	if t.ticker != nil {
		t.ticker.Stop()
	}
	if t.duration > 0 {
		t.state = StateCountdown
		t.ticker = time.NewTicker(1 * time.Second)
		go t.run()
	} else {
		t.state = StateIdle
	}
}

func (t *Timer) GetStatus() TimerStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return TimerStatus{State: t.state, Duration: t.duration}
}

// sendNotification sends a desktop notification using notify-send.
func (t *Timer) sendNotification(title, message string) {
	cmd := exec.Command("notify-send", title, message)
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending notification: %v\n", err)
		// Consider logging the error to a file
	}
}

// ----  Server-Specific Code ----

func handleConnection(conn net.Conn, timer *Timer) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req Request
	if err := decoder.Decode(&req); err != nil {
		response := Response{Success: false, Message: "Invalid request format."}
		encoder.Encode(response) // Send error response
		return
	}

	var response Response
	switch req.Type {
	case RequestTypeStatus:
		status := timer.GetStatus()
		response = Response{Success: true, Status: status}
	case RequestTypeAddSeconds:
		seconds, err := strconv.Atoi(req.Payload)
		if err != nil {
			response = Response{Success: false, Message: "Invalid seconds value."}
		} else {
			timer.AddSeconds(seconds)
			response = Response{Success: true, Message: fmt.Sprintf("Added %d seconds.", seconds)}
		}
	case RequestTypeReset: // Handle the reset request
		timer.Reset()
		response = Response{Success: true, Message: "Timer reset."}

	default:
		response = Response{Success: false, Message: "Unknown request type."}
	}

	if err := encoder.Encode(response); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding response: %v\n", err)
	}
}

func main() {
	// Get initial duration from command-line arguments (optional)
	initialDuration := 0 * time.Second
	if len(os.Args) > 1 {
		durationStr := os.Args[1]
		duration, err := time.ParseDuration(durationStr) // Parse as a duration string
		if err != nil {
			fmt.Println("duration:", err)
			fmt.Println("Invalid duration format. Using 0s.")
		} else {
			initialDuration = duration
		}
	}
	timer := NewTimer(initialDuration)
	timer.Start()

	// Remove any existing socket file
	os.Remove(SocketPath)

	listener, err := net.Listen("unix", SocketPath)
	if err != nil {
		fmt.Println("Error listening:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Server listening on", SocketPath)

	// Graceful shutdown on interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("Shutting down server...")
		listener.Close() // Close the listener to stop accepting new connections
		os.Exit(0)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Handle listener closed error during shutdown
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				return // Exit the loop if the listener is closed
			}
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleConnection(conn, timer)
	}
}


package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

type State string

const (
	StateCountdown State = "countdown"
	StateIdle      State = "idle"
	SocketPath           = "/tmp/pomidoras.sock" // Must match the server's socket path
)

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

func main() {
	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		os.Exit(1)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	var req Request
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-a":
			if len(os.Args) < 3 {
				fmt.Println("Usage: pomidoras_client -a <seconds>")
				os.Exit(1)
			}
			req = Request{Type: RequestTypeAddSeconds, Payload: os.Args[2]}
		case "-r": // Handle reset flag
			req = Request{Type: RequestTypeReset}
		default:
			fmt.Println("Invalid argument.")
			os.Exit(1)
		}
	} else {
		req = Request{Type: RequestTypeStatus}
	}

	if err := encoder.Encode(&req); err != nil {
		fmt.Println("Error sending request:", err)
		os.Exit(1)
	}

	var resp Response
	if err := decoder.Decode(&resp); err != nil {
		fmt.Println("Error receiving response:", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Println("Server error:", resp.Message)
		os.Exit(1)
	}

	if req.Type == RequestTypeStatus {
		if resp.Status.State == StateCountdown {
			minutes := int(resp.Status.Duration.Minutes())
			seconds := int(resp.Status.Duration.Seconds()) % 60
			fmt.Printf("%02d:%02d\n", minutes, seconds)
		} else {
			fmt.Println("Idle")
		}
	} else {
		fmt.Println(resp.Message) // Print server's success/failure message
	}
}


package main

import (
	"fmt"
	"os"
	"os/exec" // Import the exec package
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

type State string

const (
	StateCountdown State = "countdown"
	StateIdle      State = "idle"
)

type Timer struct {
	duration        time.Duration
	initialDuration time.Duration
	state           State
	ticker          *time.Ticker
	mu              sync.RWMutex
	terminalWidth   int
}

func NewTimer(initialDuration time.Duration) *Timer {
	state := StateIdle
	if initialDuration > 0 {
		state = StateCountdown
	}

	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}

	return &Timer{
		duration:        initialDuration,
		initialDuration: initialDuration,
		state:           state,
		terminalWidth:   width,
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
		t.clearLine()
		fmt.Println("Idle")
	}
	t.setupSignalHandlers()
}

func (t *Timer) run() {
	for range t.ticker.C {
		t.mu.Lock()
		t.duration -= time.Second
		if t.duration <= 0 {
			t.state = StateIdle
			t.ticker.Stop()
			t.duration = 0
			t.clearLine()
			fmt.Println("Idle")
			t.sendNotification("Timer Finished", "Congrats, you should stop now") // Send notification
			t.mu.Unlock()
			return
		}
		minutes := int(t.duration.Minutes())
		seconds := int(t.duration.Seconds()) % 60
		output := fmt.Sprintf("%02d:%02d", minutes, seconds)
		t.clearLine()
		fmt.Println(output)
		t.mu.Unlock()
	}
}

func (t *Timer) clearLine() {
	// fmt.Printf("\r%s", fmt.Sprintf("%*s", t.terminalWidth, "")) // Clear the line.
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
	t.clearLine()
	fmt.Println("Added", seconds, "seconds")
}

func (t *Timer) AddMinutes(minutes int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.duration += time.Duration(minutes) * time.Minute
	if t.state == StateIdle && t.duration > 0 {
		t.state = StateCountdown
		t.ticker = time.NewTicker(1 * time.Second)
		go t.run()
	}

	t.clearLine()
	fmt.Println("Added", minutes, "minutes")
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
		t.clearLine()
		fmt.Printf("Timer reset to %s\n", t.duration)

	} else {
		t.state = StateIdle
		t.clearLine()
		fmt.Println("Timer reset to Idle")
	}
}

func (t *Timer) setupSignalHandlers() {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)

	go func() {
		for sig := range signalChannel {
			switch sig {
			case syscall.SIGUSR1:
				t.AddSeconds(30)
			case syscall.SIGUSR2:
				t.AddMinutes(10)
			case syscall.SIGHUP:
				t.Reset()
			}
		}
	}()
}

// sendNotification sends a desktop notification using notify-send.
func (t *Timer) sendNotification(title, message string) {
	cmd := exec.Command("notify-send", title, message)
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending notification: %v\n", err)
		// Consider logging the error to a file, as stderr might not be visible
	}
}

func main() {
	initialDuration := 0 * time.Second
	if len(os.Args) > 1 {
		_, err := fmt.Sscan(os.Args[1], &initialDuration)
		if err != nil {
			fmt.Println("Error parsing initial duration:", err)
			os.Exit(1)
		}
	}

	timer := NewTimer(initialDuration)
	timer.Start()

	select {}
}


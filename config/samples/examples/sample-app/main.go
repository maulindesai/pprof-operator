package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // Import pprof package to register HTTP handlers
	"os"
	"runtime"
	"time"
)

// simulateLoad creates artificial CPU load
func simulateLoad() {
	// Create some CPU load
	for i := 0; i < 1000000; i++ {
		_ = fmt.Sprintf("Creating some CPU load: %d", i)
	}
}

// simulateMemoryUsage allocates memory to simulate memory usage
func simulateMemoryUsage() {
	// Allocate a large slice to consume memory
	data := make([]byte, 100*1024*1024) // 100MB
	// Do something with the data to prevent compiler optimization
	for i := 0; i < len(data); i += 1024 {
		data[i] = byte(i)
	}
	// Keep the data in memory
	runtime.KeepAlive(data)
}

func main() {
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Register a simple handler for the root path
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from pprof sample app! Visit /debug/pprof for profiling.")
	})

	// Register a handler to simulate CPU load
	http.HandleFunc("/load/cpu", func(w http.ResponseWriter, r *http.Request) {
		simulateLoad()
		fmt.Fprintf(w, "CPU load simulation completed")
	})

	// Register a handler to simulate memory usage
	http.HandleFunc("/load/memory", func(w http.ResponseWriter, r *http.Request) {
		simulateMemoryUsage()
		fmt.Fprintf(w, "Memory usage simulation completed")
	})

	// Start a goroutine to periodically simulate load
	go func() {
		for {
			simulateLoad()
			simulateMemoryUsage()
			time.Sleep(5 * time.Second)
		}
	}()

	// Start the HTTP server
	log.Printf("Starting server on :%s with pprof enabled at /debug/pprof/", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

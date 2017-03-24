// +build linux

package main

import (
	"io"
	"log"
	"os"
	
	"github.com/rdeg/loc"
)

func main() {
	// Start a go routine to handle GNSS fixes from the loc package.
	done := make(chan struct {})	// channel used to terminate locHandler
	defer close(done)
	work := loc.Init("", 0)			// let loc package determine lsdt
	defer loc.Exit()
	go locHandler(work, done)
	
	// Open the GNSS device.
	file, err := os.Open("/dev/ttyACM0")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Read the GNSS device stream and feed-back the loc package.
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
//log.Println(string(buf[:n]))
			loc.Feed(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error: read %d bytes: %v\n", n, err)
			break
		}
	}
}

// This goroutine handles the LocData sent by the loc package.
func locHandler(work chan *loc.LocInfo, done chan struct{}) {
	for {
		select {
			case li := <-work:
			log.Printf("LocInfo: %v\n\n", li)
		case <-done:
			return	// exit the handler
		}
	}
}

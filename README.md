# loc

`go get github.com/rdeg/loc`

Package loc converts an NMEA-0183 stream into compiled fixes.
At the end of each NMEA cycle, a new fix is compiled and a LocInfo structure is delivered on a channel. This channel is created on user's behalf when the Init function is called.

The LocInfo structure gives information about the quality of the fix (navigation mode, DOPs, etc.), time, actual location (latitude,
longitude, elevation), speed, heading as well as the characteristics of the satellites in view and used for the solution.

## Example usage

A typical usage would look like the following code:

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
		work := loc.Init("", 0)		// let loc package determine lsdt
		defer loc.Exit()
		go locHandler(work, done)
		
		// Open the GNSS device.
		// Please note that some serial line tuning may be needed before
		// operating the port, such as suppressing the echo and the conversion
		// of CR into LF (i.e. 'stty -F /dev/ttyACM0 -echo -icrlf').
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

package loc_test

import (
	"log"
	"os"
	"unsafe"
	
	"github.com/rdeg/loc"
)

func ExampleInit() {
	// Start a go routine to handle GNSS fixes from the loc package.
	// The channel used to retrieve fixes is returned by loc.Init.
	done := make(chan struct {})	// channel used to terminate locHandler
	defer close(done)
	work := loc.Init("", 0)			// let loc package determine lsdt
	defer loc.Exit()
	go locHandler(work, done)
}
func ExampleInit_knownLSDT() {
	done := make(chan struct {})
	defer close(done)
	work := loc.Init("GPGLL", 0)	// the NMEA cycle ends with GPGLL
	defer loc.Exit()
	go locHandler(work, done)
}
func ExampleFeed() {
	// Open the GNSS file.
	file, err := os.Open("/dev/ttyACM0")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Read the GNSS file until error or EOF, feed-back the loc package.
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
			log.Printf("read %d bytes: %v", n, err)
			break
		}
	}
}
func ExamplePack() {
	// This typically happens in the locHandler Goroutine.
	select {
	case li := <-work:
		// Pack the LocInfo we just received.
		pli := loc.Pack(li)
		
		// loc.Pack returns a slice of bytes whose payload should
		// exactly match an EBSFLocInfo structure.
		le := (*loc.EBSFLocInfo)(unsafe.Pointer(&pli[0]))
		
		// Roughly check the packed version of the LocInfo.
		if le.Utc != li.Utc || le.Lat != li.Lat || le.Lon != li.Lon || int(le.Satinfo.Inview) != len(li.Sats) {
			panic("PACKED STRUCTURE DOES'NT MATCH UNPACKED ONE!\n")
		}
		log.Printf("Packed structure (%d bytes) is OK\n\n", len(pli))
	case <-done:
		return
	}
}

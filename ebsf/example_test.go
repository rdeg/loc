package ebsf_test

import (
	"log"
	"unsafe"
	
	"github.com/rdeg/loc"
	"github.com/rdeg/loc/ebsf"
)

func ExamplePack() {
	// This typically happens in the locHandler Goroutine.
	select {
	case li := <-work:
		// Pack the loc.LocInfo we just received.
		pli := ebsf.Pack(li)
		
		// loc.Pack returns a slice of bytes whose payload should
		// exactly match an EBSFLocInfo structure.
		eli := (*ebsf.EBSFLocInfo)(unsafe.Pointer(&pli[0]))
		
		// Roughly check the packed version of the LocInfo.
		if eli.Utc != li.Utc || eli.Lat != li.Lat || eli.Lon != li.Lon ||
				int(eli.Satinfo.Inview) != len(li.Sats) {
			panic("PACKED STRUCTURE DOES'NT MATCH UNPACKED ONE!\n")
		}
		log.Printf("Packed structure (%d bytes) is OK\n\n", len(pli))
	case <-done:
		return
	}
}

# ebsf

`go get github.com/rdeg/loc/ebsf`

Package ebsf converts the compiled fixes given by the loc package into packed structures suitable for the wire.

See https://godoc.org/github.com/rdeg/loc and https://godoc.org/github.com/rdeg/loc/ebsf documentation for details.

## Example usage

A typical usage would look like the following code:

	import (
		"log"
		"unsafe"
		
		"github.com/rdeg/loc"
		"github.com/rdeg/loc/ebsf"
	)
	
	.
	.
	.

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

## Credits

This package is based on work carried out at ACTIA PCs (http://www.actia-pcs.fr/en/)
as part of the EBSF 2 project (http://ebsf2.eu/).

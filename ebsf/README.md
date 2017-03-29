# ebsf

`go get github.com/rdeg/loc/ebsf`

Package ebsf converts the compiled fixes given by the loc package into packed structures suitable for the wire.

See https://godoc.org/github.com/rdeg/loc and https://godoc.org/github.com/rdeg/loc/ebsf documentation for details.

## Example usage

A typical usage would look like the following code:

	import (
		"unsafe"
		
		"github.com/rdeg/loc"
		"github.com/rdeg/loc/ebsf"
	)
	.
	.
	.
	
	// This goroutine handles the LocInfo sent by the loc package.
	func locHandler(work chan *loc.LocInfo, done chan struct{}) {
		for {
			select {
			case li := <-work:
				// Pack the loc.LocInfo we just received.
				pli := ebsf.Pack(li)

				// loc.Pack returns a slice of bytes whose payload should
				// exactly match an EBSFLocInfo structure.
				eli := (*ebsf.EBSFLocInfo)(unsafe.Pointer(&pli[0]))

				// Do something clever with the EBSFLocInfo.
				.
				.
				.

			case <-done:
				return
			}
		}
	}
	.
	.
	.


## Credits

This package is based on work carried out at ACTIA PCs (http://www.actia-pcs.fr/en/)
as part of the EBSF 2 project (http://ebsf2.eu/).

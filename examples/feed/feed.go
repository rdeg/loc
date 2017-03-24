package main

import (
	"bufio"
	"flag"
	"fmt"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"
	
	"github.com/rdeg/loc"
)

//\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/
// This goroutine handles the LocData sent by the loc package.
//
func locHandler(work chan *loc.LocInfo, done chan struct{}) {
	for {
		select {
		case li := <-work:
//fmt.Printf("Fix: %v (%T)\n", li, li)
			// Count in-use satellites
			inuse := 0
			for _, v := range li.Sats {
				if v.Inuse {
					inuse++
				}
			}
			
			// Display a summary.
			if li.Level > loc.LOC_HAVE_NOTHING {
				fmt.Printf("%02d:%02d:%02d: Lat = %f, Lon = %f, Quality = %d, Mode = %d, HDOP = %f, Level = %d\n",
							li.Utc.Hour, li.Utc.Minute, li.Utc.Second, li.Lat, li.Lon, li.Quality, li.NavMode, li.Hdop, li.Level)
				fmt.Printf("          Sats (*%d/%d): ", inuse, len(li.Sats))
				for _, v := range li.Sats {
					if v.Inuse {
						fmt.Print("*")
					}
					fmt.Printf("%d ", v.Id)
				}
				fmt.Printf("\n")
				
			} else { // not even RMC
				fmt.Printf("%02d:%02d:%02d: NO FIX, In View = %d\n",
							li.Utc.Hour, li.Utc.Minute, li.Utc.Second, len(li.Sats));
			}
			
			// Check the packed version of the LocInfo.
			pli := loc.Pack(li)	// []byte
			le := (*loc.EBSFLocInfo)(unsafe.Pointer(&pli[0]))	// *loc.EBSFLocInfo
			if le.Level != li.Level || le.Quality != li.Quality || le.NavMode != li.NavMode || le.Smask != li.Smask ||
			   le.Utc != li.Utc || le.Pdop != li.Pdop || le.Hdop != li.Hdop || le.Vdop != li.Vdop ||
			   le.Lat != li.Lat || le.Lon != li.Lon || le.Elv != li.Elv || le.Speed != li.Speed ||
			   le.Heading != li.Heading || le.Heading != li.Heading || le.Mv != li.Mv ||
			   int(le.Satinfo.Inuse) != inuse || int(le.Satinfo.Inview) != len(li.Sats) {
				panic(fmt.Sprintf("PACKED STRUCTURE DOES'NT MATCH!\n%s\n", hex.Dump(pli)))
			}
			fmt.Printf("Packed structure (%d bytes) is OK\n\n", len(pli))
		case <-done: // exit the handler
			return
		}
	}
}

// - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
// Validate an NMEA sentence (without its terminating CR and LF).
// Return the coma-separated fields if the sentence looks OK.
func splitSentence(sentence []byte) (ss []string, ok bool) {
	if len(sentence) == 0 {	// empty line
		return
	}
	if sentence[0] != '$' {
		fmt.Printf("%s: Missing leading '$'\n", string(sentence))
		return
	}
	n := len(sentence)
	if n < 3 + 5 { // should be refined!
		fmt.Printf("%s: sentence is too short (%d bytes)!\n", string(sentence), n)
		return
	}
	if n > 80 {
		fmt.Printf("%s: sentence is too long (%d bytes)!\n", string(sentence), n)
		return
	}
	m := n - 3	// new len if a checksum is present
	if sentence[m] == '*' { // checksum
		var ccs byte	// computed checksum
		for i := 1; i < m; i++ {	// ]'$'..'*'[]
			ccs ^= sentence[i]
		}
		scs, _ := strconv.ParseUint(string(sentence[m + 1: m + 3]), 16, 8)
		if byte(scs) != ccs {
			fmt.Printf("%s: bad checksum: %02X != %02X\n", string(sentence), scs, ccs)
			return
		}
		n = m	// checksum OK: take this new length
	}

	// Extract all the fields, skipping leading '$'.
	ss = strings.Split(string(sentence[1:n]), ",")
	if len(ss) < 2 {
		fmt.Printf("%s: sentence seems invalid (%v)!\n", string(sentence), ss)
		return
	}
	
	ok = true
	return
}

// - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
// Read a line-based stream and try to retrieve the characteristics of the
// NMEA cycle.
// Return the cycle period and the Data Type of the last cycle sentence.
func getCycle(scanner *bufio.Scanner) (period int, lsdt string) {
	var psdt string		// previous sequence data type
	var prevSec int		// current second, previous second
	var nOk int			// counter of valid first sequences
	
	// Read every line of the file.
	// [CR]LF sentence termination is stripped in the scanned result.
	for scanner.Scan() {
		ss, ok := splitSentence(scanner.Bytes())
		if !ok {
			continue
		}
		
		// The GGA, RMC and GLL sentences can have a time in it.
		// Beware of Void RMC and GLL...
		sf := string(ss[0][2:])	// Sequence Formatter
//fmt.Printf("%s, ", sf)
		var st string
		var haveTime bool
		switch sf {
		case "RMC":
			if ss[2] == "A" {	// active RMC
				st = ss[1]
				haveTime = true
			} else {
				nOk = 0
			}
		case "GGA":
			st = ss[1]
			haveTime = true
		case "GLL":
			if ss[6] == "A" {	// active RMC
				st = ss[5]
				haveTime = true
			} else {
				nOk = 0
			}
		}
		if haveTime {
			if st != "" && len(st) >= 6 {	// hhmmss.ss expected
				curSec, _ := strconv.Atoi(st[4:6])
//fmt.Printf("%s (%d, %d), ", ss[0], curSec, nOk)
				if curSec != prevSec {	// assume the start of a new cycle
					ps := curSec - prevSec
					if ps < 0 {	// e.g. 0 - 59 == -59
						ps += 60
					}
					if uint(ps) > 60 {
						fmt.Printf("\n%s: invalid time!\n", st)
						nOk = 0
						continue
					}
					prevSec = curSec
					nOk++
					if nOk == 10 {
						period = ps * 1000	// ms
						lsdt = psdt	// return the data type of the previous sentence
fmt.Printf("\nperiod = %d, first = %s, last = %s\n", period, ss[0], lsdt)
						return	// period and lsdt
					}
				}
			}
		}		
		psdt = ss[0]	// potential lsdt
	}

	return	// period, lsdt probably empty, 0
}

// -------------------------------------------------------------------------
// The main function.
func main() {
	var olsdt string
	var operiod int
	
	// Retrieve command-line flags.
	flag.IntVar(&operiod, "period", 0, "specify the NMEA cycle period, in milliseconds")
	flag.StringVar(&olsdt, "lsdt", "", "give the data type of the last sequence in the cycle (e.g. 'GPRMC')")
	flag.Parse()

	// We expect as the first argument the name of a file containing an NMEA log.
	if len(flag.Args()) == 0 {
		flag.Usage()
		return
	}
	file, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	// Get a Scanner in order to read the file on a line-per-line basis.
	scanner := bufio.NewScanner(file)
	
	// Try to determine the characteristics of the NMEA sentences cycle.
	period, lsdt := getCycle(scanner)
	if operiod != 0 {
		period = operiod	// prefer user-given period
	}
	if olsdt != "" {
		lsdt = olsdt			// prefer user-given LSDT
	}
	if period == 0 || lsdt == "" {
		return
	}
fmt.Printf("period = %d, lsdt = %s\n", period, lsdt)

	// Start a go routine to handle fixes from the loc package.
	// The channel used to retrieve fixes is returned by loc.Init.
	done := make(chan struct {})
	defer close(done)
	work := loc.Init(lsdt, 0)
//	work := loc.Init("", 0)		// let loc package determine lsdt
	defer loc.Exit()
	go locHandler(work, done)

	// Read every line of the file.
	// [CR]LF sentence termination is stripped in the scanned result.
	// Need to 'rewind' the scan...
	file.Seek(0, 0)
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		ss, ok := splitSentence(scanner.Bytes())
		if !ok {
			continue
		}
		fmt.Println(scanner.Text())
		loc.Feed(append(scanner.Bytes(), '\r', '\n'))
		if ss[0] == lsdt {
			// Sleep during period ms at the end of the cycle.
			// If the cycle ends with a GSV, we have to check that
			// the sentence is the last one in the GSV burst.
			sf := ss[0][2:]	// Sequence Formatter
			if sf != "GSV" || (sf == "GSV" && (ss[1] == ss[2])) {
				fmt.Println()
				time.Sleep(time.Duration(period) * time.Second / 1000)
			}
		}
	}
}
/*
	ss := []string{
		"toto$GPGSA,A",
		",3,04,05,,",
		"09,12,,,24,,,,,2.5,1.3,2.1*39\r\n$GPGSA,A,3,04,05,,09,12,,,24,,,,,2.5,1.3,2.1*39\r\n",	"$GPGSV,2,1,08,01,40,083,46,02,17,308,41,12,07,344,39,14,22,228,45*75\r\n$GPGLL,4916.45,N,12311.12,W,225444,A,*1D\r\n$GPVTG,054.7,T,034.4,M,005.5,N,010.2,K*48\r\n",
		"$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47\r\n",
		"$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A\r\n",
		"$GPGSV,2,1,08,02,43,088,38,04,42,145,00,05,11,291,00,07,60,043,35*71\r\n",
		"$GPGSV,2,2,08,08,02,145,00,09,46,303,47,24,16,178,32,26,18,231,43*77\r\n",		"$GPRMC,162254.00,A,3723.02837,N,12159.39853,W,0.820,188.36,110706,,,A*74\r\n$GPVTG,188.36,T,,M,0.820,N,1.519,K,A*3F\r\n$GPGGA,162254.00,3723.02837,N,12159.39853,W,1,03,2.36,525.6,M,-25.6,M,,*65\r\n$GPGSA,A,2,25,01,22,,,,,,,,,,2.56,2.36,1.00*02\r\n$GPGSV,4,1,14,25,15,175,30,14,80,041,,19,38,259,14,01,52,223,18*76\r\n$GPGSV,4,2,14,18,16,079,,11,19,312,,14,80,041,,21,04,135,25*7D\r\n$GPGSV,4,3,14,15,27,134,18,03,25,222,,22,51,057,16,09,07,036,*79\r\n$GPGSV,4,4,14,07,01,181,,15,25,135,*76\r\n$GPGLL,3723.02837,N,12159.39853,W,162254.00,A,A*7C\r\n$GPZDA,162254.00,11,07,2006,00,00*63\r\n",
	}
	for _, s := range ss {
		Feed([]byte(s))
	}
*/

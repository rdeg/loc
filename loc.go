package loc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	// Level of information available in the LocInfo structure (level)
	LOC_HAVE_NOTHING    = 0 // nothing yet
	LOC_HAVE_TIME       = 1 // UTC time, after RMC showing valid time and date fields
	LOC_HAVE_POSITION   = 2 // + lat, lon, speed, direction, declination (nice RMC)
	LOC_HAVE_ALTITUDE   = 3 // + elv (nice GGA)
	LOC_HAVE_DOP        = 4 // + DOPs and active satellites (nice GSA)
	LOC_HAVE_SATELLITES = 5 // + sats (after GSV)

	// Fix quality indicator (quality, from GGA.Quality)
	LOC_SIG_BAD  = 0 // no fix/invalid
	LOC_SIG_GPS  = 1 // standard GPS (2D/3D)
	LOC_SIG_DGPS = 2 // differential GPS
	LOC_SIG_DR   = 6 // dead reckonning

	// Navigation mode (navMode, from GSA.NavMode)
	LOC_FIX_NONE = 0 // no position fix
	LOC_FIX_BAD  = 1 // no position fix
	LOC_FIX_2D   = 2 // 2D fix
	LOC_FIX_3D   = 3 // 3D fix

	// Types of parsed NMEA packets parsed
	GxNON = 0x0000 // Unknown packet type.
	GxGGA = 0x0001 // GGA - Essential fix data which provide 3D location and accuracy data.
	GxGSA = 0x0002 // GSA - GPS receiver operating mode, SVs used for navigation, and DOP values.
	GxRMC = 0x0004 // RMC - Recommended Minimum Specific GPS/TRANSIT Data.
	GxVTG = 0x0008 // VTG - Actual track made good and speed over ground.
	GxGSV = 0x0010 // GSV - Number of satellites in view, PRN numbers, elevation, azimuth & SNR values.
	GLGSV = 0x0020 // like GPGSV but for Glonass satellites
	GAGSV = 0x0040 // like GPGSV but for Galileo satellites

	GSA_MAXSAT = 12 // max sats in a GSA message
)

// Time structure.
// The LocTime structure is an equivalent of the Windows SYSTEMTIME structure,
// described in the MSDN (https://msdn.microsoft.com/fr-fr/library/windows/desktop/ms724950(v=vs.85).aspx).
type LocTime struct {
	Year   uint16
	Month  uint16
	Dow    uint16 // Day Of the Week
	Day    uint16
	Hour   uint16
	Minute uint16
	Second uint16
	Ms     uint16
}

// Information about a satellite.
type LocSat struct {
	Id      uint8  // Satellite ID
	Elv     uint8  // Elevation in degrees, 90 maximum
	Azimuth uint16 // Azimuth, degrees from true north, 000 to 359
	Sig     uint8  // Signal, 00-99 dB
	Inuse   bool   // Used in position fix
}

// Location information.
type LocInfo struct {
	Level   uint8    // Level of information available for this fix (see the LOC_HAVE_XXX constants)
	Quality uint8    // GPS quality indicator (0 = Invalid; 1 = Fix; 2 = Differential, 3 = Sensitive)
	NavMode uint8    // Operating mode, used for navigation (1 = Fix not available; 2 = 2D; 3 = 3D)
	Smask   uint8    // NMEA sentences processed for this fix
	Utc     LocTime  // UTC of position
	Pdop    float32  // Position Dilution Of Precision
	Hdop    float32  // Horizontal Dilution Of Precision
	Vdop    float32  // Vertical Dilution Of Precision
	Lat     float32  // Latitude
	Lon     float32  // Longitude
	Elv     float32  // Antenna altitude above/below mean sea level (geoid) in meters
	Speed   float32  // Speed over the ground in kilometers/hour
	Heading float32  // Track angle in degrees True
	Mv      float32  // Magnetic variation degrees (Easterly var. subtracts from true course)
	Sats    []LocSat // Satellites information
}

// Sentence processing function and minimal validation.
type fmtS struct {
	fn	func([]string)	// processing function
	mf	int				// minimum fields in spliced sentence
}

var (
	// locChan is used to return GNSS fixes to the user. After every cycle of
	// NMEA messages, a LocInfo is delivered on this channel.
	locChan chan *LocInfo

	// curLoc is the structure where data is progressivly built.
	curLoc	LocInfo

	iuBM     [256 / 8]uint8 // bitmap of in use satellites (IDs 0..256, 0 unused)
	lastGSV  bool           // true when the last GSV message of a burst has been read
	noGSVcnt uint           // successive fixes without GSV message

	// Used for the determination of sentence type at the end of the NMEA cycle.
	lst    string    		// Last Sentence Type
	pst    string    		// previous sentence type
	tst    string    		// temporary lst (copied to lst when nOk is 4)
	nOk    int       		// # successive cycles ending with lst
	minDel time.Duration    // minimum delay between two cycles (in ns)
	tPrev  time.Time	 	// Time of previous sentence

	// Sentence processing functions.
	// We assume that we can ignore the Talker ID (i.e. "GP", "GL", "GA",
	// "GB" or "GN") in the Address field and focus on the Sentence Formatter
	// that follows ("GGA", "RMC", "VTG", "GSV").
	// This is made possible only when the GNSS receiver ensures that
	// different satellite numbering ranges are used for different satellite
	// constellations.
	fmtFA = map[string]fmtS{
		"GGA": {doGGA, 10},
		"GSA": {doGSA, 18},
		"RMC": {doRMC, 12},
		"GSV": {doGSV, 4},
		//"VTG":{doVTG,	9},	// not useful if RMC is OK
	}

	// Used by Feed to isolate sentences from the stream.
	feedState int = 0	// frame decoding state
	feedBuf []byte		// intermediate sentence buffer
)

/*
// Compute the number of in-use satellites.
func nInUse(loc *LocInfo) (n uint16) {
	for _, v := range loc.Sats {
		if v.Inuse {
			n++
		}
	}
	return
}
*/

// Return a copy of curLoc and reset curLoc for the next fix.
func getLoc() *LocInfo {
	// Compute the 'level'
	curLoc.Level = LOC_HAVE_NOTHING // assume we have nothing serious
	if curLoc.Smask&GxRMC != 0 {    // RMC
		if curLoc.Quality != LOC_SIG_BAD { // Active RMC (valid fix)
			if curLoc.Smask&GxGSA != 0 { // Active RMC, GSA
				if curLoc.Smask&GxGSV != 0 || len(curLoc.Sats) != 0 { // Active RMC, GSA, GSV
					curLoc.Level = LOC_HAVE_SATELLITES // 5
				} else { // Active RMC, GSA
					curLoc.Level = LOC_HAVE_DOP // 4
				}
			} else { // Active RMC
				if curLoc.Smask&GxGGA != 0 { // Active RMC, GGA
					curLoc.Level = LOC_HAVE_ALTITUDE // 3
				} else { // Active RMC alone
					curLoc.Level = LOC_HAVE_POSITION // 2
				}
			}
		} else { // Void RMC (invalid fix)
			if curLoc.Utc.Year != 0 {
				curLoc.Level = LOC_HAVE_TIME // 1
			}
		}
	}

	// Here is the fix!
	lastLoc := curLoc // *allocate* and copy everything

	// Prepare data structures for the next fix.
	//	curLoc = LocInfo{}	// clear the working LocInfo
	// On some GNSS receptors, GSV data is not delivered on every fix, so we
	// have to preserve satellite information until a terminal GSV message is
	// received after the delivery of this fix.
	// So, clear all but curLoc.Sats.
	curLoc.Level = 0
	curLoc.Quality = 0
	curLoc.NavMode = 0
	curLoc.Smask = 0
	curLoc.Utc = LocTime{}
	curLoc.Pdop = 0
	curLoc.Hdop = 0
	curLoc.Vdop = 0
	curLoc.Lat = 0
	curLoc.Lon = 0
	curLoc.Elv = 0
	curLoc.Speed = 0
	curLoc.Heading = 0
	curLoc.Mv = 0

	// If we have 5 consecutive fixes without GSV message, clear curLoc.Sats.
	if lastLoc.Smask&GxGSV != 0 { // we had GSV for this fix
		noGSVcnt = 0
	} else { // no GSV here
		noGSVcnt++
		if noGSVcnt >= 4 {
			curLoc.Sats = []LocSat{}
		}
	}
	//fmt.Printf("Smask = 0x%02x, noGSVcnt = %d\n", lastLoc.Smask, noGSVcnt)

	// Clear the in-use satellites bitmap.
	// We assume that GSA information will be delivered for each fix.
	//for _, v := range iuBM {fmt.Printf("%02X ", v)};fmt.Println()
	iuBM = [256 / 8]uint8{}

//fmt.Println("FIX")
	// Return a reference to the allocated LocInfo.
	return &lastLoc
}

// Fix latitude or longitude.
// Input: lat or lon string in degrees and minutes ("ddmm.mmmmm" or "dddmm.mmmmm")
// Output: same value in degrees (dd.dddddd or ddd.dddddd)
func fixLG(lg float64) float32 {
	i, f := math.Modf(float64(lg) / 100) // i = dd.0, f = .mmmmmmmm
	return float32(i + f*100/60)
}

// Compute the day of the week from current date.
// Credits to Tomohiko Sakamoto in sci.math.
func fixDow(stm *LocTime) {
	y := stm.Year
	if stm.Month < 3 {
		y--
	}
	stm.Dow = (uint16)((y + y/4 - y/100 + y/400 + uint16("-bed=pen+mad."[stm.Month]) + stm.Day) % 7)
}

// GGA: Global positionning system fix data
func doGGA(fields []string) {
	fmt.Sscanf(fields[1], "%2d%2d%2d.%2d", // hhmmss.ss
		&curLoc.Utc.Hour, &curLoc.Utc.Minute, &curLoc.Utc.Second, &curLoc.Utc.Ms)
	curLoc.Utc.Ms *= 10

	lat, _ := strconv.ParseFloat(fields[2], 32) // ddmm.mmmmm
	curLoc.Lat = fixLG(lat)
	if fields[3] == "S" {
		curLoc.Lat = -curLoc.Lat
	}

	lon, _ := strconv.ParseFloat(fields[4], 32) // dddmm.mmmmm
	curLoc.Lon = fixLG(lon)
	if fields[5] == "W" {
		curLoc.Lon = -curLoc.Lon
	}

	q, _ := strconv.Atoi(fields[6])
	curLoc.Quality = uint8(q)

	h, _ := strconv.ParseFloat(fields[8], 32) // HDOP (also in GSA)
	curLoc.Hdop = float32(h)

	a, _ := strconv.ParseFloat(fields[9], 32) // alt(itude)
	curLoc.Elv = float32(a)

	curLoc.Smask |= GxGGA
}

// RMC: Recommended Minimum data
func doRMC(fields []string) {
	fmt.Sscanf(fields[1], "%2d%2d%2d.%2d", // hhmmss.ss
		&curLoc.Utc.Hour, &curLoc.Utc.Minute, &curLoc.Utc.Second, &curLoc.Utc.Ms)
	curLoc.Utc.Ms *= 10

	lat, _ := strconv.ParseFloat(fields[3], 32) // ddmm.mmmmm
	curLoc.Lat = fixLG(lat)
	if fields[4] == "S" {
		curLoc.Lat = -curLoc.Lat
	}

	lon, _ := strconv.ParseFloat(fields[5], 32) // dddmm.mmmmm
	curLoc.Lon = fixLG(lon)
	if fields[6] == "W" {
		curLoc.Lon = -curLoc.Lon
	}

	speed, _ := strconv.ParseFloat(fields[7], 32) // speed over ground (knots)
	curLoc.Speed = float32(speed * 1.852)         // km/h

	heading, _ := strconv.ParseFloat(fields[8], 32) // course over ground (degrees)
	curLoc.Heading = float32(heading)

	fmt.Sscanf(fields[9], "%2d%2d%2d", // ddmmyy
		&curLoc.Utc.Day, &curLoc.Utc.Month, &curLoc.Utc.Year)
	curLoc.Utc.Year += 2000
	fixDow(&curLoc.Utc) // set the day of the week

	mv, _ := strconv.ParseFloat(fields[10], 32) // magnetic variation (degrees)
	curLoc.Mv = float32(mv)
	if fields[11] == "W" { // @@@ NOT SURE OF THIS!
		curLoc.Mv = -curLoc.Mv
	}

	switch fields[2] { // status
	case "A": // Active
		if curLoc.Quality == LOC_SIG_BAD {
			curLoc.Quality = LOC_SIG_GPS // assume it will be fixed with GGA.Quality
		}
		if curLoc.NavMode <= LOC_FIX_BAD { // LOC_FIX_NONE and LOC_FIX_BAD
			curLoc.NavMode = LOC_FIX_2D // assume it will be fixed with GSA.NavMode
		}
	case "V": // Void
		curLoc.Quality = LOC_SIG_BAD
		curLoc.NavMode = LOC_FIX_BAD
	}

	curLoc.Smask |= GxRMC
}

/*
// VTG: course over ground and ground speed
func doVTG(fields []string) {
	if fields[2] == "T" {
		h, _ := strconv.ParseFloat(fields[1], 32)	// course over ground (true) (degrees)
		curLoc.heading = float32(h)
	}
	if fields[8] == "K" {
		k, _ := strconv.ParseFloat(fields[7], 32)	// speed over ground (km/h)
		curLoc.speed = float32(k)
	} else {
		if fields[6] == "N" {
			k, _ := strconv.ParseFloat(fields[5], 32)	// speed over ground (knots)
			curLoc.speed = float32(k * 1.852)			// km/h
	}

	curLoc.Smask |= GxVTG
}
*/

// GSA: DOP and active satellites
func doGSA(fields []string) {
	m, _ := strconv.Atoi(fields[2]) // navMode
	curLoc.NavMode = uint8(m)

	// Get the ids of the satellites used for navigation.
	// GPS, Galileo and Glonass GGA messages can be processed here.
	// GPS satellites are numbered from 1 to 32.
	// Glonass satellites are numbered from 65 to 96.
	// Beidou satellites are (possibly) numbered from 201 to 235.
	// Galileo satellites numbering is currently unknown.
	// We just expect that the final overall numbering will be consistent
	// enough to avoid collisions!
	for i := 3; i < 3+12; i++ { // 12 satellites maximum per GSA sentence
		id, _ := strconv.Atoi(fields[i]) // satellite number (1-255 expected)
		if id == 0 {
			continue
		}
		if id > 255 {
			panic(fmt.Sprintf("UNEXPECTED SATELLITE NUMBER IN %s: %d!", fields[0], id))
		}
		iuBM[id/8] |= 1 << (uint)(id%8) // 8-bit per iuBM entry
	}

	// Get the DOPs now.
	pdop, _ := strconv.ParseFloat(fields[15], 32) // PDOP
	curLoc.Pdop = float32(pdop)
	hdop, _ := strconv.ParseFloat(fields[16], 32) // HDOP (also in GGA)
	curLoc.Hdop = float32(hdop)
	vdop, _ := strconv.ParseFloat(fields[17], 32) // VDOP
	curLoc.Vdop = float32(vdop)

	curLoc.Smask |= GxGSA
}

// GSV: Satelites in View
func doGSV(fields []string) {
	// Retrieve a few values from the message.
	numMsg, _ := strconv.Atoi(fields[1]) // expected number of GSV messages
	msgNum, _ := strconv.Atoi(fields[2]) // number of this message (1,2,3)
	numSV, _ := strconv.Atoi(fields[3])  // number of satellites in view

	// Compute the number of satellites detailed in this message.
	// If msgNum < numMsg, it should be 4.
	// If msgNum == numMsg, it should be 1 + numSV % 4.
	// Please note that a GSV message with NO satellite is possible, like in
	// the "$GPGSV,1,1,00*79" that can be returned by ublox NEO-M8.
	if msgNum > numMsg || numSV < 0 || numMsg != (numSV+3)/4 {
		fmt.Printf("Invalid GSV message: %v\n", fields)
		return
	}
	ns := 4               // assume full
	if msgNum == numMsg { // last message
		ns = (numSV-1)%4 + 1 // 1, 2, 3, 4 (0 when numSV is 0)
	}
	fmt.Printf("numMsg = %d, msgNum = %d, numSV = %d, ns = %d\n", numMsg, msgNum, numSV, ns)
	if len(fields) < 4 + 4 * ns {
		fmt.Printf("%d < %d\n", len(fields), 4 + 4 * ns)
		fmt.Printf("Invalid GSV message: %v\n", fields)
		return
	}

	// Consider cleaning curLoc.Sats if a 'first' GSV message comes after a
	// 'final' GSV message, i.e. if a new GPGSV set begins.
	// The lastGSV boolean is true right after the 'final' GSV message has
	// been processed (see bellow).
	// WARNING: it is here assumed that if GxGSV messages are issued for
	// several constellations, the first one for the GPS (i.e. GPGSV).
	 // dont reset Sats if not GPGSV
	if lastGSV && fields[0][1] == 'P' {
		lastGSV = false
		curLoc.Sats = []LocSat{}
	}

	// Check whether this is the last GSV sentence of a GSV burst.
	if msgNum == numMsg { // last GSV sentence of a GSV burst
		lastGSV = true
	}

	// Append given satellite information to curLoc.Sats.
	var ls LocSat
	for i := 0; i < ns; i++ {
		sv, _ := strconv.Atoi(fields[4+i*4+0]) // satelite ID
		if sv == 0 || uint(sv) > 255 {
			fmt.Printf("UNEXPECTED SATELLITE NUMBER IN %s: %02d\n", fields[0], sv)
			return
		}
		elv, _ := strconv.Atoi(fields[4+i*4+1]) // elevation
		az, _ := strconv.Atoi(fields[4+i*4+2])  // azimuth
		cno, _ := strconv.Atoi(fields[4+i*4+3]) // signal strength

		ls.Id = uint8(sv)
		ls.Elv = uint8(elv)
		ls.Azimuth = uint16(az)
		ls.Sig = uint8(cno)
		ls.Inuse = (iuBM[sv/8] & (1 << (uint(sv) % 8))) != 0

		curLoc.Sats = append(curLoc.Sats, ls) // len(curLoc.Sats) gives the number of satellites in view
	}

	curLoc.Smask |= GxGSV
}

// Check if the given (spliced) sentence is the last one in the NMEA cycle.
func checkCycle(ss []string) bool {
	if lst != "" { // known Last Sequence Data Type
		if ss[0] == lst { // match
			// If the cycle ends with a GSV, we have to check that
			// the sentence is the last one in the GSV burst.
			sf := ss[0][2:] // Sequence Formatter
			return sf != "GSV" || (sf == "GSV" && (ss[1] == ss[2]))
		}
	} else { // try to determine lst
		if pst != "" { // we have received a sentence before (tPrev.IsZero() is false)
			if time.Since(tPrev) >= minDel { // delay big enough
				// pst is a candidate
				if pst == tst {
					nOk++
					if nOk == 4 { // consecutive matches
						lst = pst // voila!
						fmt.Printf("\n*** lst = %s ***\n\n", lst)
					}
				} else {
					tst = pst
					nOk = 0
				}
			}
		}
	}
	return false
}

func cleanS(sentence []byte) string {
	n := len(sentence)
	for n != 0 {
		if sentence[n - 1] >= ' ' {	// not a control char (CR and LF)
			break
		}
		n--
	}
	return string(sentence[:n])
}

// Process an NMEA-183 sentence.
// Expected: '$...,...,...,...,... * H1 H2 CR LF'
//   len +                        -5 -4 -3 -2 -1
func processSentence(sentence []byte) {
	//fmt.Printf("\nSentence: %s", string(sentence))
	// First make some validation.
	n := len(sentence)
	if n < 3+8 { // should be refined!
		fmt.Printf("%s: sentence too short (%d bytes)!\n", cleanS(sentence), n)
		return
	}
	if n > 82 {
		fmt.Printf("%s: sentence too long (%d bytes)!\n", cleanS(sentence), n)
		return
	}
	if sentence[n-2] != '\r' {
		fmt.Printf("%s: missing CR!\n", cleanS(sentence))
		return
	}
	n -= 2                  // ignore trailing CR+LF
	m := n - 3              // new len if a checksum is present
	if sentence[m] == '*' { // checksum
		var ccs byte             // computed checksum
		for i := 1; i < m; i++ { // ]'$'..'*'[]
			ccs ^= sentence[i]
		}
		scs, _ := strconv.ParseUint(string(sentence[m+1:m+3]), 16, 8)
		if byte(scs) != ccs {
			fmt.Printf("%s: bad checksum: %02X != %02X\n", cleanS(sentence), scs, ccs)
			return
		}
		n = m // checksum OK: take this new length
	}

	// We have useful material in sentence[1:n].
	//fmt.Printf("Sentence: %s\n", sentence[1:n])

	// Extract all the fields, skipping leading '$'.
	ss := strings.Split(string(sentence[1:n]), ",")
	//fmt.Println(ss)

	// Keep on determining the end of the NMEA cycle.
	// Do this before checking the Sequence Formatter, as the cycle
	// can be terminated by a sentence we don't process.
	eoc := checkCycle(ss)

	// Save the time when we received this sentence and save its type as
	// the "previous sequence type".
	tPrev = time.Now()
	pst = ss[0]

	// Keep on processing according to the Sequence Formatter.
	// Ignore the 2-letters Target ID that precede it.
	fmts, ok := fmtFA[ss[0][2:]]
	if ok {
		if len(ss) < fmts.mf {
			fmt.Printf("%s: invalid sentence (not enough fields)\n", cleanS(sentence))
		} else {
//fmt.Printf("Processing %s (%v)\n", ss[0], ss)
			fmts.fn(ss)
		}
	} else {
		//fmt.Printf("Skipping %s\n", ss[0])
	}

	// Consider delivering a fix if a cycle has been completed.
	if eoc { // end of cycle
		locChan <- getLoc()
		//fmt.Println(lastLoc)
	}
}

// Feed should be called everytime a chunck of NMEA-183 data has been read
// from the GNSS serial or USB port.
//
// Expected frames start with a '$' sign and end with a CR LF sequence.
//
// Chunck size does not matter: Feed can accept several sentences in a row
// as well as partial sentences.
func Feed(data []byte) {
	var i int
	//fmt.Printf("Feed('%s'", data)

	for len(data) != 0 {
		switch feedState {
		case 0: // waiting for '$'
			if i = strings.IndexByte(string(data), '$'); i == -1 {
				return // ignore this data chunk
			}
			data = data[i:] // what's left
			feedState = 1	// wait for LF now

		case 1: // waiting for LF
			if i = strings.IndexByte(string(data), '\n'); i == -1 {
				feedBuf = append(feedBuf, data...) // append this chunk to the temporary buffer
				return		// stay in state 1
			}
			processSentence(append(feedBuf, data[:i+1]...)) // got a sentence
			feedBuf = feedBuf[:0]	// clear feedBuf
			data = data[i+1:]		// go on with what's left
			feedState = 0			// waiting for '$' now
		}
	}
}

// Init initializes the communication channel used to deliver compiled GNSS
// fixes to the package user.
//
// The lsdt parameter, if non empty, gives the Data Type (e.g. "GPGLL") of
// the last NMEA sentence in a cycle. If lsdt is empty, the package will try
// to determine its value by analyzing the inter-sentence delay of a few
// cycles.
//
// minDelay, if not 0, can help the package to determine the "lsdt": if the
// delay between two NMEA sentences exceeds minDelay, it is assumed that the
// cycle is over and that the lsdt is the Data Type of the previous sentence.
// The value used by default for the minimal inter-cycle delay is 300 ms.
// If lsdt is non-empty, minDelay is ignored.
//
// The Data Type of the last sentence of the NMEA cycle is determined after
// 4 consecutive exceedances of the minimal inter-cycle delay, coming after
// sentences of the same type.
//
// Init returns the initialized channel.
func Init(lsdt string, minDelay uint) chan *LocInfo {
	lst = lsdt
	
	if minDelay == 0 {
		minDel = time.Duration(300) * time.Millisecond	// default to 300 ms
	} else {
		minDel = time.Duration(minDelay) * time.Millisecond // convert ms to ns
	}

	locChan = make(chan *LocInfo)
	//fmt.Println("loc.Init() done")
	return locChan
}

// Exit undo what Init did.
func Exit() {
	close(locChan)
	//fmt.Println("loc.Exit() done")
}

package ebsf

import (
	"bytes"
	"encoding/binary"
//	"fmt"
//	"unsafe"

	"github.com/rdeg/loc"
)

const (
	EBSF_MAXSAT = 32 // maximum satellites in satinfo
)

// Type EBSFLocSat is a loc.LocSat equivalent.
type EBSFLocSat struct {
	Id      uint16 // Satellite ID
	Elv     uint8  // Elevation in degrees, 90 maximum
	Res     uint8  // reserved for alignment
	Azimuth uint16 // Azimuth, degrees from true north, 000 to 359
	Sig     uint8  // Signal, 00-99 dB
	Inuse   uint8  // Used in position fix
}

// Type EBSFLocInfo is an equivalent of the loc.LocInfo structure.
// It is returned by Pack in a []byte.
type EBSFLocInfo struct {
	Level   uint8       // Level of information available
	Quality uint8       // GPS quality indicator (0 = Invalid; 1 = Fix; 2 = Differential, 3 = Sensitive)
	NavMode uint8       // Operating mode, used for navigation (1 = Fix not available; 2 = 2D; 3 = 3D)
	Smask   uint8       // NMEA sentences processed for this fix
	Utc     loc.LocTime	// UTC of position
	Pdop    float32     // Position Dilution Of Precision
	Hdop    float32     // Horizontal Dilution Of Precision
	Vdop    float32     // Vertical Dilution Of Precision
	Lat     float32     // Latitude
	Lon     float32     // Longitude
	Elv     float32     // Antenna altitude above/below mean sea level (geoid) in meters
	Speed   float32     // Speed over the ground in kilometers/hour
	Heading float32     // Track angle in degrees True
	Mv      float32     // Magnetic variation degrees (Easterly var. subtracts from true course)
	Satinfo struct {    // Satellites information
		Inuse  uint16               // Number of satellites in use (not those in view)
		Inview uint16               // Total number of satellites in view
		Sat [EBSF_MAXSAT]EBSFLocSat // Per-satellite information
	}
}

/*
Pack packs a loc.LocInfo structure into an EBSF LOCINFO, suited for the wire.

In C notation, the EBSF LOCSATELLITE and LOCINFO structures have the
following binary layout:

	// Information about a satellite.
	typedef struct {
	    unsigned short id;        // Satellite ID
	    unsigned char  elv;       // Elevation in degrees, 90 maximum
	    unsigned char  reserved;  // alignment
	    unsigned short azimuth;   // Azimuth, degrees from true north, 000 to 359
	    unsigned char  sig;       // Signal, 00-99 dB
	    unsigned char  in_use;    // Used in position fix
	} LOCSAT;                     // 8 bytes

	// Location information.
	typedef struct {
	    unsigned char  bLevel;    // 00: level of information available
	    unsigned char  bQuality;  // 01: GPS quality indicator (0 = Invalid; 1 = Fix; 2 = Differential, 3 = Sensitive)
	    unsigned char  bNavMode;  // 02: Operating mode, used for navigation (1 = Fix not available; 2 = 2D; 3 = 3D)
	    unsigned char  bRes;      // 03: reserved
	    SYSTEMTIME     utc;       // 04: UTC of position
	    float          PDOP;      // 20: Position Dilution Of Precision
	    float          HDOP;      // 24: Horizontal Dilution Of Precision
	    float          VDOP;      // 28: Vertical Dilution Of Precision
	    float          lat;       // 32: Latitude
	    float          lon;       // 36: Longitude
	    float          elv;       // 40: Antenna altitude above/below mean sea level (geoid) in meters
	    float          speed      // 44: Speed over the ground in kilometers/hour
	    float          heading;   // 48: Track angle in degrees True
	    float          mv;        // 52: Magnetic variation degrees (Easterly var. subtracts from true course)
	    struct {                  // 56: Information about all visible satellites.
	     unsigned short inuse;    // 56: Number of satellites in use (not those in view)
	     unsigned short inview;   // 58: Total number of satellites in view
	     LOCSAT sat[EBSF_MAXSAT]; // 60:Satellites information
	    }              satinfo;   // 260 bytes (2 + 2 + 32 * 8)
	} LOCINFO;                    // 316 bytes (56 + 260)

The result is returned in a slice of exactly 316 bytes.

Please note that the number of satellites that the satinfo field of a LOCINFO
can hold is limited to 32 (EBSF_MAXSAT). This is not critical when a pure GPS
receptor is used but it is theoretically possible that more satellites
can be seen when a GNSS receptor able to handle multiple constellations is
used. To address this possibility, Pack copies the in-use satellites first,
then the other satellites, within the limit of 32. 
*/
func Pack(li *loc.LocInfo) []byte {
	var buf bytes.Buffer
	var eli EBSFLocInfo

	eli.Level = li.Level
	eli.Quality = li.Quality
	eli.NavMode = li.NavMode
	eli.Smask = li.Smask
	eli.Utc = li.Utc
	eli.Pdop = li.Pdop
	eli.Hdop = li.Hdop
	eli.Vdop = li.Vdop
	eli.Lat = li.Lat
	eli.Lon = li.Lon
	eli.Elv = li.Elv
	eli.Speed = li.Speed
	eli.Heading = li.Heading
	eli.Mv = li.Mv

	copySat := func(esat *EBSFLocSat, sat *loc.LocSat) {
		esat.Id = uint16(sat.Id)
		esat.Elv = sat.Elv
		//		esat.Res		= 0
		esat.Azimuth = sat.Azimuth
		esat.Sig = sat.Sig
		if sat.Inuse {
			esat.Inuse = 1
		} else {
			esat.Inuse = 0
		}
	}

	// Copy in-use satellites first, then copy the other satellites.
	// We cannot copy the info of more than EBSF_MAXSAT satellites.
	eli.Satinfo.Inview = 0
	for i := range li.Sats {
		if li.Sats[i].Inuse {
			copySat(&eli.Satinfo.Sat[eli.Satinfo.Inview], &li.Sats[i])
			eli.Satinfo.Inuse++
			eli.Satinfo.Inview++
			if eli.Satinfo.Inview == EBSF_MAXSAT {
				goto copydone
			}
		}
	}
	for i := range li.Sats {
		if !li.Sats[i].Inuse {
			copySat(&eli.Satinfo.Sat[eli.Satinfo.Inview], &li.Sats[i])
			eli.Satinfo.Inview++
			if eli.Satinfo.Inview == EBSF_MAXSAT {
				goto copydone
			}
		}
	}
copydone:

	binary.Write(&buf, binary.LittleEndian, eli)
//fmt.Println("sizeof(eli) =", unsafe.Sizeof(eli), "len(buf.Bytes()) =", len(buf.Bytes()))
//fmt.Println("outbuf =", buf.Bytes())
	return buf.Bytes()
}
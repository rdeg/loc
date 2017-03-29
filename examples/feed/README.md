# feed.go

This program illustrates the use of loc and loc/ebsf packages.

It also provides a way to replay previously saved raw NMEA data, which can be useful for the development of similar programs.

## Usages

Replay NMEA1.LOG at its initial speed. The file is pre-analysed and fixes are immediately produced:

	go run feed.go NMEA1.LOG

Replay NMEA2.LOG at maximum speed. The file is pre-analysed and fixes are immediately produced:

	go run feed.go -period 0 NMEA2.LOG

Replay NMEA3.LOG at half-speed. No pre-analyse takes place. Fixes are immediately produced:

	go run feed.go -lsdt=GPRMC -period=500 NMEA3.LOG

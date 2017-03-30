# real.go

This program is an example of using the loc package in a real environment.
It was deliberately left simple to illustrate the operation of an application that reads directly from a serial link the data of a GPS/GNSS receiver, feeds loc with this data and retrieves the compiled 'fixes'.

As such, it only works on Linux or any other OS that makes it so easy to read a serial link in Go language.
Be careful, however: some tuning may be needed before operating the serial port, such as setting the baud rate, suppressing the echo or inhibiting the conversion of CR into LF (e.g. 'stty -F /dev/ttyACM0 -echo -icrlf').

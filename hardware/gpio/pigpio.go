package gpio

import (
	"encoding/binary"
	"fmt"
	"net"
)

// Pigpio is used for controlling GPIO over the pigpio socket interface
type Pigpio struct {
	conn net.Conn
}

// compile-time check for whether Pigpio satisfies the GPIO interface
var _ GPIO = &Pigpio{}

// DialPigpio dials into the pigpio socket interface (normally running on port 8888)
func DialPigpio(addr string) (*Pigpio, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("couldn't dial into pigpio socket: %w", err)
	}

	return &Pigpio{conn: conn}, nil
}

// Close closes the underlying pigpio socket interface connection
func (p *Pigpio) Close() error {
	if p.conn == nil {
		return fmt.Errorf("connection is already closed")
	}

	return p.conn.Close()
}

// Write sets a GPIO pin to LOW or HIGH.
func (p *Pigpio) Write(pin int, level Level) error {
	if p.conn == nil {
		return fmt.Errorf("not connected to pigpio socket interface")
	}

	var rawLevel uint32
	if level {
		rawLevel = 1
	}

	return p.writeGPIO(uint32(pin), rawLevel)
}

// PWM sets frequency and duty cycle for hardware PWM on the given pin.
func (p *Pigpio) PWM(pin int, frequency int, duty float64) error {
	if p.conn == nil {
		return fmt.Errorf("not connected to pigpio socket interface")
	}

	return p.hp(uint32(pin), uint32(frequency), uint32(float64(1000000)*duty))
}

type cmd struct {
	Cmd uint32
	P1  uint32
	P2  uint32
	P3  uint32
}

const (
	read  uint32 = 3
	write uint32 = 4
	hp    uint32 = 86
)

func (p *Pigpio) writeGPIO(pin, level uint32) error {
	request := cmd{
		Cmd: write,
		P1:  pin,
		P2:  level,
	}

	if err := binary.Write(p.conn, binary.LittleEndian, request); err != nil {
		return fmt.Errorf("unable to write request to socket: %w", err)
	}

	var response cmd
	if err := binary.Read(p.conn, binary.LittleEndian, &response); err != nil {
		return fmt.Errorf("unable to read response from socket: %w", err)
	}

	return nil
}

// hp sets frequency (1-125,000,000) and duty cycle (1-1000000) for hardware PWM on the specified pin.
func (p *Pigpio) hp(pin, frequency, duty uint32) error {
	request := struct {
		Cmd uint32
		P1  uint32
		P2  uint32
		P3  uint32
		Ext uint32
	}{
		Cmd: hp,
		P1:  pin,
		P2:  frequency,
		P3:  4,
		Ext: duty,
	}

	if err := binary.Write(p.conn, binary.LittleEndian, request); err != nil {
		return fmt.Errorf("unable to write request to socket: %w", err)
	}

	var response cmd
	if err := binary.Read(p.conn, binary.LittleEndian, &response); err != nil {
		return fmt.Errorf("unable to read response from socket: %w", err)
	}

	return nil
}

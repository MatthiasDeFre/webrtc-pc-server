package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

const (
	FramePacketType   uint32 = 0
	ControlPacketType uint32 = 1
)

type RemoteInputPacketHeader struct {
	Framenr     uint32
	Framelen    uint32
	Frameoffset uint32
	Packetlen   uint32
}

type RemoteFrame struct {
	currentLen uint32
	frameLen   uint32
	frameData  []byte
}

type ProxyConnection struct {
	// General
	addr *net.UDPAddr
	conn *net.UDPConn

	// Receiving
	m                 sync.RWMutex
	incomplete_frames map[uint32]RemoteFrame
	complete_frames   []RemoteFrame
	frameCounter      uint32
	peerConnections   map[uint64]*PeerConnection
	indi_mode         bool
}

func NewProxyConnection(peerConnections map[uint64]*PeerConnection, indi_mode bool) *ProxyConnection {
	return &ProxyConnection{nil, nil, sync.RWMutex{}, make(map[uint32]RemoteFrame), make([]RemoteFrame, 0), 0, peerConnections, indi_mode}
}

func (pc *ProxyConnection) sendPacket(b []byte, offset uint32, packet_type uint32) {
	buffProxy := make([]byte, 1500)
	binary.LittleEndian.PutUint32(buffProxy[0:], packet_type)
	copy(buffProxy[4:], b[offset:])
	_, err := pc.conn.WriteToUDP(buffProxy, pc.addr)
	if err != nil {
		fmt.Println("Error sending response:", err)
		panic(err)
	}
}

func (pc *ProxyConnection) SetupConnection(port string) {
	address, err := net.ResolveUDPAddr("udp", port)
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}

	// Create a UDP connection
	pc.conn, err = net.ListenUDP("udp", address)
	if err != nil {
		fmt.Println("Error listening:", err)
		return
	}
	println(pc.conn.LocalAddr().Network(), pc.conn.LocalAddr().String(), address.String())
	// Create a buffer to read incoming messages
	buffer := make([]byte, 1500)

	// Wait for incoming messages
	fmt.Println("Waiting for a message...")
	_, pc.addr, err = pc.conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Println("Error reading:", err)
		return
	}
	fmt.Println("Connected to proxy")
}

func (pc *ProxyConnection) StartListening() {
	println("listen")
	str := "Hello!"
	byteArray := make([]byte, 1500)
	copy(byteArray[:], str)
	//byteArray[len(str)] = 0
	_, err := pc.conn.WriteToUDP(byteArray, pc.addr)
	if err != nil {
		fmt.Println("Error sending response:", err)
		return
	}
	println("sending")
	go func() {
		for {
			buffer := make([]byte, 1500)
			_, _, _ = pc.conn.ReadFromUDP(buffer)
			var packetType uint32
			err := binary.Read(bytes.NewReader(buffer[:4]), binary.BigEndian, &packetType)
			bufBinary := bytes.NewBuffer(buffer[4:20])
			// Read the fields from the buffer into a struct
			if packetType == 2 && pc.indi_mode == false {
				var p RemoteInputPacketHeader
				err = binary.Read(bufBinary, binary.LittleEndian, &p)
				if err != nil {
					fmt.Println("Error:", err)
					return
				}
				pc.m.Lock()
				_, exists := pc.incomplete_frames[p.Framenr]
				if !exists {
					r := RemoteFrame{
						0,
						p.Framelen,
						make([]byte, p.Framelen),
					}
					pc.incomplete_frames[p.Framenr] = r
				}
				value := pc.incomplete_frames[p.Framenr]

				copy(value.frameData[p.Frameoffset:p.Frameoffset+p.Packetlen], buffer[20:20+p.Packetlen])
				value.currentLen = value.currentLen + p.Packetlen
				pc.incomplete_frames[p.Framenr] = value

				if value.currentLen == value.frameLen {
					if p.Framenr%5 == 0 {
						println("REMOTE FRAME ", p.Framenr, " COMPLETE")
					}
					pc.complete_frames = append(pc.complete_frames, value)
					delete(pc.incomplete_frames, p.Framenr)
				}
				//println(p.Frameoffset, p.Framenr, value.currentLen, p.Framelen)
				pc.m.Unlock()
			} else if packetType == 2 {

			} else {
				pc.SendBitrates()
			}

		}
	}()
}
func (pc *ProxyConnection) SendFramePacket(b []byte, offset uint32) {
	pc.sendPacket(b, offset, FramePacketType)
}

func (pc *ProxyConnection) SendBitrates() bool {
	nClients := uint64(len(pc.peerConnections))
	buffer := new(bytes.Buffer)
	err := binary.Write(buffer, binary.LittleEndian, nClients)
	if err != nil {
		return false
	}
	for key, peerConn := range pc.peerConnections {
		// Write the key to the buffer
		err := binary.Write(buffer, binary.LittleEndian, key)
		if err != nil {
			return false
		}

		// Write the PeerConnection.GetBitrate() to the buffer
		err = binary.Write(buffer, binary.LittleEndian, peerConn.GetBitrate())
		if err != nil {
			return false
		}
	}
	pc.sendPacket(buffer.Bytes(), 0, 1)
	return true
}

func (pc *ProxyConnection) NextFrame() []byte {
	isNextFrameReady := false
	for !isNextFrameReady {
		pc.m.Lock()
		if len(pc.complete_frames) > 0 {
			isNextFrameReady = true
		}
		pc.m.Unlock()
	}
	pc.m.Lock()
	data := pc.complete_frames[0].frameData
	if pc.frameCounter%100 == 0 {
		println("SENDING FRAME ", pc.frameCounter)
	}
	pc.complete_frames = pc.complete_frames[1:]
	pc.frameCounter = pc.frameCounter + 1
	pc.m.Unlock()
	return data
}

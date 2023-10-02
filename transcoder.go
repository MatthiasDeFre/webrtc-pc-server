package main

import (
	"bytes"
	"encoding/binary"
	"time"
)

type Frame struct {
	ClientID uint32
	FrameLen uint32
	FrameNr  uint32
	Data     []byte
}

type Transcoder interface {
	UpdateBitrate(bitrate uint32)
	UpdateProjection()
	EncodeFrame(data []byte) *Frame
	IsReady() bool
	GetEstimatedBitrate() uint32
	GetFrameCounter() uint32
	IncrementFrameCounter()
	NextFrame() []byte
}

type TranscoderFiles struct {
	frames           []FileData
	frameCounter     uint32
	isReady          bool
	fileCounter      uint32
	lEnc             *LayeredEncoder
	estimatedBitrate uint32
	prevFrameTime    int64
	frameRate        uint32
}

func (f *Frame) Bytes() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, f.ClientID)
	binary.Write(buf, binary.BigEndian, f.FrameLen)
	binary.Write(buf, binary.BigEndian, f.FrameNr)
	binary.Write(buf, binary.BigEndian, f.Data)
	return buf.Bytes()
}

func NewTranscoderFile(contentDirectory string, frameRate uint32) *TranscoderFiles {
	fBytes, _ := ReadBinaryFiles(contentDirectory)
	return &TranscoderFiles{fBytes, 0, true, 0, NewLayeredEncoder(), 0, 0, frameRate}
}

func (t *TranscoderFiles) UpdateBitrate(bitrate uint32) {
	t.estimatedBitrate = bitrate
	t.lEnc.Bitrate = bitrate
}

func (t *TranscoderFiles) UpdateProjection() {
	// Do nothing
}

func (t *TranscoderFiles) NextFrame() []byte {
	sleepTime := int64(1000/t.frameRate) - (time.Now().UnixMilli() - t.prevFrameTime)
	if sleepTime > 0 {
		time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	}
	t.prevFrameTime = time.Now().UnixMilli()
	return t.frames[t.fileCounter].Data
}

func (t *TranscoderFiles) EncodeFrame(data []byte) *Frame {

	transcodedData := t.lEnc.EncodeMultiFrame(data)
	if data == nil {
		return nil
	}
	rFrame := Frame{0, uint32(len(transcodedData)), t.frameCounter, transcodedData}
	t.frameCounter = (t.frameCounter + 1)
	t.fileCounter = (t.fileCounter + 1) % uint32(len(t.frames))
	return &rFrame
}

func (t *TranscoderFiles) IsReady() bool {
	return t.isReady
}

func (t *TranscoderFiles) GetEstimatedBitrate() uint32 {
	return t.estimatedBitrate
}

func (t *TranscoderFiles) GetFrameCounter() uint32 {
	return t.frameCounter
}
func (t *TranscoderFiles) IncrementFrameCounter() {
	t.frameCounter++
}

type TranscoderRemote struct {
	proxyConn        *ProxyConnection
	frameCounter     uint32
	isReady          bool
	lEnc             *LayeredEncoder
	estimatedBitrate uint32
}

func NewTranscoderRemote(proxy_con *ProxyConnection) *TranscoderRemote {
	return &TranscoderRemote{proxy_con, 0, true, NewLayeredEncoder(), 0}
}

func (t *TranscoderRemote) UpdateBitrate(bitrate uint32) {
	t.estimatedBitrate = bitrate
	t.lEnc.Bitrate = bitrate
}

func (t *TranscoderRemote) UpdateProjection() {
	// Do nothing
}
func (t *TranscoderRemote) NextFrame() []byte {
	return proxyConn.NextFrame()
}

func (t *TranscoderRemote) EncodeFrame(data []byte) *Frame {
	transcodedData := t.lEnc.EncodeMultiFrame(data)
	if data == nil {
		return nil
	}
	rFrame := Frame{0, uint32(len(transcodedData)), t.frameCounter, transcodedData}
	return &rFrame
}

func (t *TranscoderRemote) IsReady() bool {
	return t.isReady
}
func (t *TranscoderRemote) GetEstimatedBitrate() uint32 {
	return t.estimatedBitrate
}
func (t *TranscoderRemote) GetFrameCounter() uint32 {
	return t.frameCounter
}
func (t *TranscoderRemote) IncrementFrameCounter() {
	t.frameCounter++
}

type TranscoderDummy struct {
	proxy_con        *ProxyConnection
	frameCounter     uint32
	isReady          bool
	bitrate          uint32
	isFixed          bool
	isDummy          bool
	estimatedBitrate uint32
}

func NewTranscoderDummy(proxy_con *ProxyConnection, bitrate uint32, isFixed bool, isDummy bool) *TranscoderDummy {
	return &TranscoderDummy{proxy_con, 0, true, bitrate, isFixed, isDummy, 0}
}

func (t *TranscoderDummy) UpdateBitrate(bitrate uint32) {
	// Do nothing
	if !t.isFixed {
		t.bitrate = bitrate
	}

}

func (t *TranscoderDummy) UpdateProjection() {
	// Do nothing
}

func (t *TranscoderDummy) EncodeFrame(data []byte) *Frame {

	if t.isDummy {
		return nil
	}
	//	//println(100000 / 8 / t.n_tiles)
	transcodedData := make([]byte, uint32(float64(t.bitrate/8/30)))
	rFrame := Frame{0, uint32(len(transcodedData)), t.frameCounter, transcodedData}
	t.frameCounter++
	return &rFrame
}

func (t *TranscoderDummy) IsReady() bool {
	return true
}
func (t *TranscoderDummy) GetEstimatedBitrate() uint32 {
	return t.estimatedBitrate
}
func (t *TranscoderDummy) GetFrameCounter() uint32 {
	return t.frameCounter
}
func (t *TranscoderDummy) IncrementFrameCounter() {
	t.frameCounter++
}
func (t *TranscoderDummy) NextFrame() []byte {
	return make([]byte, uint32(float64(t.bitrate/8/30)))
}

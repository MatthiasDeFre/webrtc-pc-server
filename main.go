package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

const (
	Idle     int = 0
	Hello    int = 1
	Offer    int = 2
	Answer   int = 3
	Ready    int = 4
	Finished int = 5
)

var clientCounter uint64
var proxyConn *ProxyConnection
var peerConnections map[uint64]*PeerConnection
var api *webrtc.API
var nClients int
var frameResultwriter *FrameResultWriter

func main() {
	useVirtualWall := flag.Bool("v", false, "Use virtual wall ip filter")
	proxyPort := flag.String("p", ":0", "Use as a proxy with specified port")
	contentDirectory := flag.String("d", "content_jpg", "Content directory")
	contentFrameRate := flag.Int("f", 30, "Frame rate that is used when using files instead of proxy")
	signallingIP := flag.String("s", "127.0.0.1:5678", "Signalling server IP")
	numberOfClients := flag.Int("c", 1, "Number of clients")
	resultDirectory := flag.String("m", "", "Result directory")
	flag.Parse()
	frameResultwriter = NewFrameResultWriter(*resultDirectory, 5)
	fileCont, _ := os.OpenFile(*resultDirectory+"_cont.csv", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	fileCont.WriteString("time;estimated_bitrate;loss_rate;delay_rate;loss\n")
	print(*useVirtualWall)
	nClients = *numberOfClients
	useProxy := false
	if *proxyPort != ":0" {
		proxyConn = NewProxyConnection()
		fmt.Println(*proxyPort)
		proxyConn.SetupConnection(*proxyPort)
		useProxy = true

	}
	var transcoder Transcoder
	if useProxy {
		transcoder = NewTranscoderRemote(proxyConn)
	} else {
		transcoder = NewTranscoderFile(*contentDirectory, uint32(*contentFrameRate))
	}
	// TODO Transcoder layered
	clientCounter = 0
	peerConnections = make(map[uint64]*PeerConnection)
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetSCTPMaxReceiveBufferSize(16 * 1024 * 1024)

	i := &interceptor.Registry{}
	m := NewMediaEngine()
	// Sender side

	congestionController, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(gcc.SendSideBWEMinBitrate(75_000*8), gcc.SendSideBWEInitialBitrate(75_000_000), gcc.SendSideBWEMaxBitrate(262_744_320))
	})
	if err != nil {
		panic(err)
	}

	congestionController.OnNewPeerConnection(func(id string, estimator cc.BandwidthEstimator) {
		println("NEW BW ESTIMATOR")
		peerConnections[clientCounter-1].estimator = estimator
	})

	i.Add(congestionController)
	if err = webrtc.ConfigureTWCCHeaderExtensionSender(m, i); err != nil {
		panic(err)
	}

	responder, _ := nack.NewResponderInterceptor()
	i.Add(responder)

	generator, err := twcc.NewSenderInterceptor(twcc.SendInterval(10 * time.Millisecond))
	if err != nil {
		panic(err)
	}

	i.Add(generator)

	nackGenerator, _ := nack.NewGeneratorInterceptor()
	i.Add(nackGenerator)
	api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine), webrtc.WithInterceptorRegistry(i), webrtc.WithMediaEngine(m))
	//api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine), webrtc.WithInterceptorRegistry(i), webrtc.WithMediaEngine(m))
	//api = NewWebrtcAPI(peerConnections)

	defer func() {
		// CLOSE ALL PEER CONNECTIONS
		for _, p := range peerConnections {
			if cErr := p.webrtcConnection.Close(); cErr != nil {
				fmt.Printf("Cannot close peer connection: %v\n", cErr)
			}
		}
	}()
	// CHANGE TO SERVER

	var state = Idle
	println("Current state:", state)
	NewWSServer(*signallingIP, wsNewUserCb)
	// Infinite loop sending aggregate frames every 33ms

	//select {}
	for {
		frameData := transcoder.NextFrame()
		for _, pc := range peerConnections {
			// Get frame from proxy = channel (maybe ring channel)
			if pc.isReady {
				//transcoder.UpdateBitrate(uint32(75_000_000))
				if transcoder.GetFrameCounter()%5 == 0 {
					vLossRate, _ := pc.estimator.GetStats()["lossTargetBitrate"]
					vDelayRate, _ := pc.estimator.GetStats()["delayTargetBitrate"]
					vLoss, _ := pc.estimator.GetStats()["averageLoss"]
					timestamp := time.Now().UnixNano() / int64(time.Millisecond)
					data := fmt.Sprintf("%d;%d;%d;%d;%.2f\n", timestamp, uint32(pc.estimator.GetTargetBitrate()), vLossRate, vDelayRate, vLoss)
					fileCont.WriteString(data)
				}
				transcoder.UpdateBitrate(uint32(pc.estimator.GetTargetBitrate()))
				if frame := transcoder.EncodeFrame(frameData); frame != nil {
					frameResultwriter.CreateRecord(transcoder.GetFrameCounter(), time.Now().UnixNano()/int64(time.Millisecond), true)
					frameResultwriter.SetEstimatedBitrate(transcoder.GetFrameCounter(), uint32(pc.estimator.GetTargetBitrate()))
					frameResultwriter.SetSizeInBytes(transcoder.GetFrameCounter(), uint32(len(frameData)), true)

					pc.track.WriteFrame(frame)
					if pc.track.currentFrame%100 == 0 {
						println("MULTIFRAME", pc.track.currentFrame, pc.clientID, len(frame.Data))
					}

					frameResultwriter.SetProcessingCompleteTimestamp(transcoder.GetFrameCounter(), time.Now().UnixNano()/int64(time.Millisecond), true)
					frameResultwriter.SaveRecord(transcoder.GetFrameCounter(), true)
				}
			}
		}
		if transcoder.GetFrameCounter()%100 == 0 {
			println("FRAME", transcoder.GetFrameCounter(), "send to all clients")

		}

		/*if transcoder.GetFrameCounter() == 99 {
			println("FRAME", transcoder.GetFrameCounter(), "send to all clients")
			time.Sleep(10 * time.Second)
			os.Exit(0)
		}*/
		transcoder.IncrementFrameCounter()

	}
}
func getCodecCapability() webrtc.RTPCodecCapability {
	videoRTCPFeedback := []webrtc.RTCPFeedback{
		{Type: "goog-remb", Parameter: ""},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"},
	}

	return webrtc.RTPCodecCapability{
		MimeType:     "video/pcm",
		ClockRate:    90000,
		Channels:     0,
		SDPFmtpLine:  "",
		RTCPFeedback: videoRTCPFeedback,
	}
}
func NewMediaEngine() *webrtc.MediaEngine {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: getCodecCapability(),
		PayloadType:        5,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeVideo)
	if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}

	m.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeAudio)
	if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}
	return m
}

func NewWebrtcAPI(peerConnections map[uint64]*PeerConnection) *webrtc.API {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetSCTPMaxReceiveBufferSize(16 * 1024 * 1024)

	i := &interceptor.Registry{}
	m := NewMediaEngine()
	// Sender side
	congestionController, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(gcc.SendSideBWEMinBitrate(75_000*8), gcc.SendSideBWEInitialBitrate(75_000_000), gcc.SendSideBWEMaxBitrate(262_744_320))
	})
	if err != nil {
		panic(err)
	}

	congestionController.OnNewPeerConnection(func(id string, estimator cc.BandwidthEstimator) {
		println("NEW BW ESTIMATOR")
		peerConnections[clientCounter-1].estimator = estimator
	})

	if err = webrtc.ConfigureTWCCHeaderExtensionSender(m, i); err != nil {
		panic(err)
	}

	responder, _ := nack.NewResponderInterceptor()

	generator, err := twcc.NewSenderInterceptor(twcc.SendInterval(10 * time.Millisecond))
	if err != nil {
		panic(err)
	}

	nackGenerator, _ := nack.NewGeneratorInterceptor()

	i.Add(congestionController)
	i.Add(responder)
	i.Add(generator)
	i.Add(nackGenerator)

	return webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine), webrtc.WithInterceptorRegistry(i), webrtc.WithMediaEngine(m))
}

func wsNewUserCb(wsConn *websocket.Conn) {
	peerConnections[clientCounter] = NewPeerConnection(clientCounter, wsConn, wsHandlerMessageCbFunc)
	clientCounter++
	if int(clientCounter) == nClients {
		proxyConn.StartListening()
	}
}

func wsHandlerMessageCbFunc(wsPacket WebsocketPacket) {
	switch wsPacket.MessageType {
	case 1: // hello
		println("Received hello")
		peerConnections[wsPacket.ClientID].Init(api)
	case 3: // answer
		println("Received answer")
		answer := webrtc.SessionDescription{}
		err := json.Unmarshal([]byte(wsPacket.Message), &answer)
		if err != nil {
			panic(err)
		}
		peerConnections[wsPacket.ClientID].SetRemoteDescription(answer)

	case 4: // candidate
		println("Received candidate")
		candidate := wsPacket.Message
		if candidateErr := peerConnections[wsPacket.ClientID].AddICECandidate(candidate); candidateErr != nil {
			panic(candidateErr)
		}
	default:
		println(fmt.Sprintf("Received non-compliant message type %d", wsPacket.MessageType))
	}
}

func wsMessageReceivedCb(wsPacket WebsocketPacket) {

}

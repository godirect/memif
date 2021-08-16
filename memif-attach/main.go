// Copyright (c) 2020 Cisco and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"net"
	"time"
	"unsafe"
	"encoding/binary"
	"bufio"
	"bytes"
	"git.fd.io/govpp.git/api"
	"github.com/edwarnicke/govpp/binapi/memif"
	interfaces "github.com/edwarnicke/govpp/binapi/interface"
	"github.com/edwarnicke/govpp/binapi/interface_types"
	"github.com/edwarnicke/log"
	"github.com/edwarnicke/vpphelper"
	memfd "github.com/justincormack/go-memfd"
	"syscall"
)

type memifRegionIndexT uint16
type memifRegionOffsetT uint32
type memifRegionSizeT uint64
type memifRingIndexT uint16
// Ack message type
type AckMsg struct {
	Ack uint16
}
type MemifMsg struct {
	HelloMsgType HelloMsg
	InitMsgType InitMsg
}
// HelloReplyMsg type
type HelloMsg struct {
	Ack AckMsg
	Name [32]byte // 32 bytes array
	Min_version uint16 
	Max_version uint16
	Max_region uint16 
	Max_m2s_ring uint16 
	Max_s2m_ring uint16
	Max_log2_ring_size uint8 
} 

// InitMsg
const MEMIF_SECRET_SIZE = 24
type MemifInterfaceMode int32
const (
	MEMIF_INTERFACE_MODE_ETHERNET MemifInterfaceMode = iota
	MEMIF_INTERFACE_MODE_IP // the one to create memif
	MEMIF_INTERFACE_MODE_PUNT_INJECT
)
type InitMsg struct {
	Ack AckMsg
	Version uint16 // check the file
	Id uint32 // 0
	Mode MemifInterfaceMode
	Secret  [MEMIF_SECRET_SIZE]uint8
	Name [32]byte
}

// Add Region Message
type AddRegionMsg struct {
	Ack AckMsg
	Index uint16
	Size uint64
}

// Add Ring Message
type AddRingMsg struct {
	Ack AckMsg
	Index uint16
	Region uint16
	Offset uint32
	Max_log2_ring_size uint8
	Private_hdr_size uint16
}

// memif ring t
type MemifRingT struct {
	cacheline0 uint8
	cookie uint32
	flags uint16
	head uint16
	cacheline1 uint8
	cacheline2 uint8
	desc[0] MemifDescT
}
// memif desc t
type MemifDescT struct {
	flags uint16
	region memifRegionIndexT
	length uint32
	offset memifRegionOffsetT
	metadata uint32
}
// UnmarshalBinary Function
func (replyMsg *HelloMsg) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.BigEndian, replyMsg)
	return err
}
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Connect to VPP with a 1 second timeout
	connectCtx, _ := context.WithTimeout(ctx, time.Second)
	conn, vppErrCh := vpphelper.StartAndDialContext(connectCtx)
	exitOnErrCh(ctx, cancel, vppErrCh)
	
	// Create a RPC client for the memif api
	isClient := true
	// Add a memif socket
	socketID, socketAddr, _:= createMemifSocket(ctx, conn)
	// Create a memif interface
	createMemif(ctx, conn, socketID, isClient)
	
	// Dump memif info
	dumpMemif(ctx, conn)
	
	// connect to VPP
	conn_client, err := net.Dial("unixpacket", socketAddr)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: connect to VPP master failed:", err)
	}
	// read hello message from server
	helloMsg := handleHelloMsg(ctx, conn_client)
	// send init message to server
	sendInitMsg(ctx, conn_client, helloMsg)
	cancel()
	<-vppErrCh
}

func createMemifSocket(ctx context.Context, conn api.Connection) (socketID uint32, socket_filename string, err error) {
	c := memif.NewServiceClient(conn)
	MemifSocketFilenameAddDel := memif.MemifSocketFilenameAddDel{
		IsAdd:          true,
		SocketID:       2,
		SocketFilename: "/var/run/vpp/memif11.sock",
	}
	_, memifAddDel_err := c.MemifSocketFilenameAddDel(ctx, &MemifSocketFilenameAddDel)
	if memifAddDel_err != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifSocketFilenameAddDel failed:", memifAddDel_err)
	}

	log.Entry(ctx).Infof("Socket file created\n"+
		"SocketID: %v\n"+
		"SocketFilename: %v\n"+
		"IsAdd:%v\n",
		MemifSocketFilenameAddDel.SocketID, MemifSocketFilenameAddDel.SocketFilename, MemifSocketFilenameAddDel.IsAdd)
	return MemifSocketFilenameAddDel.SocketID, MemifSocketFilenameAddDel.SocketFilename, memifAddDel_err 
}

func createMemif(ctx context.Context, conn api.Connection, socketID uint32, isClient bool) error {
	role := memif.MEMIF_ROLE_API_MASTER
	if !isClient {
		role = memif.MEMIF_ROLE_API_SLAVE
	}
	mode := memif.MEMIF_MODE_API_IP
	memifCreate := &memif.MemifCreate{
		Role:     role,
		SocketID: socketID,
		Mode:     mode,
		BufferSize: 1024,
	}
	rsp, err := memif.NewServiceClient(conn).MemifCreate(ctx, memifCreate)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: memifCreate failed:", err)
	}
	log.Entry(ctx).Infof("Memif interface created\n"+
		"swIfIndex %v\n"+
		"Role %v\n"+
		"SocketID %v\n",
		rsp.SwIfIndex, memifCreate.Role, memifCreate.SocketID)

	// Set AdminUp
	swinterface := &interfaces.SwInterfaceSetFlags {
		SwIfIndex: rsp.SwIfIndex, 
		Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP,  
	}
	retVal, err := interfaces.NewServiceClient(conn).SwInterfaceSetFlags(ctx, swinterface)
	if err != nil {
		log.Entry(ctx).Fatalln("Set AdminUp ERROR:  %v", err)
	}
	log.Entry(ctx).Infof("SwInterfaceSetFlags %v", retVal)

	return nil
}

func dumpMemif(ctx context.Context, conn api.Connection) () {
	c := memif.NewServiceClient(conn)

	memifDumpMsg, err := c.MemifDump(ctx, &memif.MemifDump{})
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifDump failed:", err)
	}
	log.Entry(ctx).Infof("Socket file dump")
	// done := make(chan bool)
	// for {
	reply, err := memifDumpMsg.Recv()
	// if err == io.EOF {
	// 	done <- true //means stream is finished
	// 		return
	// }
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifDump Recv failed:", err)
	}
	log.Entry(ctx).Infof("Socket ID from dump:%v", reply.ID)
	// }
	// <-done
	log.Entry(ctx).Infof("Finish dumping from memif")
	return 
}

func handleHelloMsg(ctx context.Context, conn_client net.Conn) (helloMsgReply *HelloMsg){
	helloMsg := &HelloMsg{}
	reader := bufio.NewReader(conn_client)
	err := binary.Read(reader, binary.BigEndian, helloMsg)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: read from VPP master hello message failed:", err)
	}
	log.Entry(ctx).Infof("max_region: %v\n"+
	"Max_m2s_ring: %v\n"+
	"Max_s2m_ring: %v\n"+
	"Min_version: %v\n"+
	"Max_version: %v\n"+
	"Max_log2_ring_size: %v\n"+
	"Name: %v\n",
	helloMsg.Max_region, helloMsg.Max_m2s_ring, helloMsg.Max_s2m_ring, helloMsg.Min_version, helloMsg.Max_version, helloMsg.Max_log2_ring_size, string(helloMsg.Name[:]))
	return helloMsg
}

func sendInitMsg(ctx context.Context, conn_client net.Conn, helloMsg *HelloMsg) {
	Ack := AckMsg {0}
	initMsg := &InitMsg{
		Ack: Ack,
		Version: ((helloMsg.Max_version<<8) | helloMsg.Min_version),
		Id: 0, // hardcoded for now
		Mode: MEMIF_INTERFACE_MODE_IP,
		Secret: [24]uint8{0},
		Name: helloMsg.Name,
	}

	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, initMsg)
	if err != nil {
		log.Entry(ctx).Fatalln("Encoding Error", err)
	}
	writer := bufio.NewWriter(conn_client)
	_, writeErr := writer.Write(buf.Bytes())
	if writeErr != nil {
		log.Entry(ctx).Fatalln("Error while writing encoded message over connection", writeErr)
	}
	log.Entry(ctx).Infof("Init message sent")
}

func createMemRegion(ctx context.Context, unixConn net.Conn, helloMsg *HelloMsg, bufferSize uint16) *memfd.Memfd{
	mfd, err := memfd.Create()
	if err != nil {
		log.Entry(ctx).Fatalln("Memory region create Error", err)
	}
	setSizeErr := mfd.SetSize(256)
	if setSizeErr != nil {
		log.Entry(ctx).Fatalln("Memory region set size Error", err)
	}
	fd := mfd.Fd()

	Ack := AckMsg {0}
	regionSize := getRegionSize(bufferSize, helloMsg)
	addRegionMsg := &AddRegionMsg{
		Ack: Ack,
		Index: uint16(fd), // should fix later
		Size: 1024, // calculate in https://github.com/FDio/vpp/blob/b8e7a45d56be9f3e11b07b82fd899160e2af1bf1/src/plugins/memif/memif.c#L376
	}
	buf := new(bytes.Buffer)
	writeErr := binary.Write(buf, binary.BigEndian, addRegionMsg)
	if writeErr != nil {
		log.Entry(ctx).Fatalln("Encoding Error", err)
	}

	rights := syscall.UnixRights(int(fd))
	unixConn.(interface {
		WriteMsgUnix(b, oob []byte, addr *net.UnixAddr) (n, oobn int, err error)
	}).WriteMsgUnix(buf.Bytes(), rights, nil)
	return mfd
}

func getRegionSize(BufferSize uint16, helloMsg *HelloMsg) (uintptr){
	memifRingT := MemifRingT{}
	memifDescT := MemifDescT{}
	bufferOffset := (uintptr(helloMsg.Max_s2m_ring + helloMsg.Max_m2s_ring)) * (unsafe.Sizeof(memifRingT)+unsafe.Sizeof(memifDescT) * (1<<helloMsg.Max_log2_ring_size))
	regionSize := uintptr(BufferSize*(1<<helloMsg.Max_log2_ring_size)*(helloMsg.Max_s2m_ring+helloMsg.Max_m2s_ring))+bufferOffset
	return regionSize
}
// func sendAddRingMsg(ctx context.Context, conn_client net.Conn, helloMsg *HelloMsg) {
// 	addRingMsg := &AddRingMsg{
// 		Index: helloMsg.Max_m2s_ring,
// 		Region: hellMsg.Max_region, 
// 		Offset: ,
// 		Max_log2_ring_size: hellMsg.Max_log2_ring_size,
// 		Private_hdr_size: 0,
// 	}

// 	buf := new(bytes.Buffer)
// 	err := binary.Write(buf, binary.BigEndian, addRegionMsg)
// 	if err != nil {
// 		log.Entry(ctx).Fatalln("Encoding Error", err)
// 	}
// 	writer := bufio.NewWriter(conn_client)
// 	_, writeErr := writer.Write(buf.Bytes())
// 	if writeErr != nil {
// 		log.Entry(ctx).Fatalln("Error while writing add ring encoded message over connection", writeErr)
// 	}
// 	log.Entry(ctx).Infof("Add ring message sent")
// }

func exitOnErrCh(ctx context.Context, cancel context.CancelFunc, errCh <-chan error) {
	// If we already have an error, log it and exit
	select {
	case err := <-errCh:
		log.Entry(ctx).Fatal(err)
	default:
	}
	// Otherwise wait for an error in the background to log and cancel
	go func(ctx context.Context, errCh <-chan error) {
		err := <-errCh
		log.Entry(ctx).Error(err)
		cancel()
	}(ctx, errCh)
}

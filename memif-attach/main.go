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
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"

	"git.fd.io/govpp.git/api"
	interfaces "github.com/edwarnicke/govpp/binapi/interface"
	"github.com/edwarnicke/govpp/binapi/interface_types"
	"github.com/edwarnicke/govpp/binapi/memif"
	"github.com/edwarnicke/log"
	"github.com/edwarnicke/vpphelper"
)

type ackMsg struct {
	Ack uint16
}

// HelloMsg type
type HelloMsg struct {
	Ack             ackMsg
	Name            [32]byte // 32 bytes array
	MinVersion      uint16
	MaxVersion      uint16
	MaxRegion       uint16
	MaxM2sRing      uint16
	MaxS2mRing      uint16
	MaxLog2RingSize uint8
}

const memifSecretSize = 24

type memifInterfaceMode int32

const (
	memifInterfaceModeIP memifInterfaceMode = iota // the one to create memif
)

// InitMsg type
type InitMsg struct {
	Ack     ackMsg
	Version uint16 // check the file
	ID      uint32 // 0
	Mode    memifInterfaceMode
	Secret  [memifSecretSize]uint8
	Name    [32]byte
}

func main() {
	ctx, cancel1 := context.WithCancel(context.Background())
	// Connect to VPP with a 1 second timeout
	connectCtx, cancel2 := context.WithTimeout(ctx, time.Second)
	conn, vppErrCh := vpphelper.StartAndDialContext(connectCtx)
	exitOnErrCh(ctx, cancel1, vppErrCh)

	// Create a RPC client for the memif api
	isClient := true
	// Add a memif socket
	socketID, socketAddr, _ := createMemifSocket(ctx, conn)
	// Create a memif interface
	createMemif(ctx, conn, socketID, isClient)

	// Dump memif info
	dumpMemif(ctx, conn)

	// connect to VPP
	connClient, err := net.Dial("unixpacket", socketAddr)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: connect to VPP master failed:", err)
	}
	// read hello message from server
	helloMsg := handleHelloMsg(ctx, connClient)
	// send init message to server
	sendInitMsg(ctx, connClient, helloMsg)
	cancel1()
	cancel2()
	<-vppErrCh
}

func createMemifSocket(ctx context.Context, conn api.Connection) (socketID uint32, socketFilename string, err error) {
	c := memif.NewServiceClient(conn)
	MemifSocketFilenameAddDel := memif.MemifSocketFilenameAddDel{
		IsAdd:          true,
		SocketID:       2,
		SocketFilename: "/var/run/vpp/memif11.sock",
	}
	_, memifAddDelErr := c.MemifSocketFilenameAddDel(ctx, &MemifSocketFilenameAddDel)
	if memifAddDelErr != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifSocketFilenameAddDel failed:", memifAddDelErr)
	}

	log.Entry(ctx).Infof("Socket file created\n"+
		"SocketID: %v\n"+
		"SocketFilename: %v\n"+
		"IsAdd:%v\n",
		MemifSocketFilenameAddDel.SocketID, MemifSocketFilenameAddDel.SocketFilename, MemifSocketFilenameAddDel.IsAdd)
	return MemifSocketFilenameAddDel.SocketID, MemifSocketFilenameAddDel.SocketFilename, memifAddDelErr
}

func createMemif(ctx context.Context, conn api.Connection, socketID uint32, isClient bool) {
	role := memif.MEMIF_ROLE_API_MASTER
	if !isClient {
		role = memif.MEMIF_ROLE_API_SLAVE
	}
	mode := memif.MEMIF_MODE_API_IP
	memifCreate := &memif.MemifCreate{
		Role:     role,
		SocketID: socketID,
		Mode:     mode,
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
	swinterface := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: rsp.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}
	retVal, err := interfaces.NewServiceClient(conn).SwInterfaceSetFlags(ctx, swinterface)
	if err != nil {
		log.Entry(ctx).Fatalln("Set AdminUp ERROR:  %v", err)
	}
	log.Entry(ctx).Infof("SwInterfaceSetFlags %v", retVal)
}

func dumpMemif(ctx context.Context, conn api.Connection) {
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
}

func handleHelloMsg(ctx context.Context, connClient io.Reader) (helloMsgReply *HelloMsg) {
	helloMsg := &HelloMsg{}
	reader := bufio.NewReader(connClient)
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
		helloMsg.MaxRegion, helloMsg.MaxM2sRing, helloMsg.MaxS2mRing, helloMsg.MinVersion, helloMsg.MaxVersion, helloMsg.MaxLog2RingSize, string(helloMsg.Name[:]))
	return helloMsg
}

func sendInitMsg(ctx context.Context, connClient io.Writer, helloMsg *HelloMsg) {
	Ack := ackMsg{0}
	initMsg := &InitMsg{
		Ack:     Ack,
		Version: ((helloMsg.MaxVersion << 8) | helloMsg.MinVersion),
		ID:      0, // hardcoded for now
		Mode:    memifInterfaceModeIP,
		Secret:  [24]uint8{0},
		Name:    helloMsg.Name,
	}

	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, initMsg)
	if err != nil {
		log.Entry(ctx).Fatalln("Encoding Error", err)
	}
	writer := bufio.NewWriter(connClient)
	_, writeErr := writer.Write(buf.Bytes())
	if writeErr != nil {
		log.Entry(ctx).Fatalln("Error while writing encoded message over connection", writeErr)
	}
	log.Entry(ctx).Infof("Init message sent")
}

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

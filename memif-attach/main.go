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
	// "io"
	"encoding/binary"
	"bufio"
	"bytes"
	"git.fd.io/govpp.git/api"
	"github.com/edwarnicke/govpp/binapi/memif"
	interfaces "github.com/edwarnicke/govpp/binapi/interface"
	"github.com/edwarnicke/govpp/binapi/interface_types"
	"github.com/edwarnicke/log"
	"github.com/edwarnicke/vpphelper"
)

// HelloReplyMsg type
type HelloMsg struct {
	max_region int16 
	max_m2s_ring int16 
	max_s2m_ring int16
	min_version int16 
	max_version int16
	max_log2_ring_size int8 
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
	mode := memif.MEMIF_MODE_API_IP
	createMemif(ctx, conn, socketID, mode, isClient)
	
	// Dump memif info
	dumpMemif(ctx, conn)
	
	// connect to VPP
	conn_client, err := net.Dial("unixpacket", socketAddr)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: connect to VPP master failed:", err)
	}
	// read hello message from master vpp
	helloMsg := &HelloMsg{}
	reader := bufio.NewReader(conn_client)
	buf, _, readErr := reader.ReadLine()
	if readErr != nil {
		log.Entry(ctx).Fatalln("Read Error", readErr)
	}

	helloErr := helloMsg.UnmarshalBinary(buf) // decode reply message
	if err != nil {
		log.Entry(ctx).Fatalln("Error while decoding data read from the vpp master", helloErr)
	}

	log.Entry(ctx).Infof("max_region: %v\n",
	helloMsg.max_region)

	cancel()
	<-vppErrCh
}

func createMemifSocket(ctx context.Context, conn api.Connection) (socketID uint32, socket_filename string, err error) {
	c := memif.NewServiceClient(conn)
	MemifSocketFilenameAddDel := memif.MemifSocketFilenameAddDel{
		IsAdd:          true,
		SocketID:       2,
		SocketFilename: "/var/run/vpp/memif4.sock",
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

func createMemif(ctx context.Context, conn api.Connection, socketID uint32, mode memif.MemifMode, isClient bool) error {
	role := memif.MEMIF_ROLE_API_MASTER
	if !isClient {
		role = memif.MEMIF_ROLE_API_SLAVE
	}
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
	swinterface := &interfaces.SwInterfaceSetFlags {
		SwIfIndex: rsp.SwIfIndex, 
		Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP,  
	}
	retVal, err := interfaces.NewServiceClient(conn).SwInterfaceSetFlags(ctx, swinterface)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR:  %v", err)
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
	reply, err := memifDumpMsg.Recv()
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifDump Recv failed:", err)
	}
	log.Entry(ctx).Infof("Socket ID from dump:%v", reply.ID)
	
	return 
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

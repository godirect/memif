// Copyright (c) 2021 Rizheng Tan and/or its affiliates.
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
	"time"

	"git.fd.io/govpp.git/api"
	interfaces "github.com/edwarnicke/govpp/binapi/interface"
	"github.com/edwarnicke/govpp/binapi/interface_types"
	"github.com/edwarnicke/govpp/binapi/memif"
	"github.com/edwarnicke/log"
	"github.com/edwarnicke/vpphelper"
)


func main() {
	ctx, cancel1 := context.WithCancel(context.Background())
	// Connect to VPP with a 1 second timeout
	connectCtx, _ := context.WithTimeout(ctx, time.Second)
	conn, vppErrCh := vpphelper.StartAndDialContext(connectCtx)
	exitOnErrCh(ctx, cancel1, vppErrCh)

	// Create a RPC client for the memif api
	isClient := true
	// Add a memif socket
	socketID, _, _ := createMemifSocket(ctx, conn)
	// Create a memif interface
	createMemif(ctx, conn, socketID, isClient)

	// Dump memif info
	dumpMemif(ctx, conn)

	// cancel1()
	// cancel2()
	// <-vppErrCh
}

func createMemifSocket(ctx context.Context, conn api.Connection) (socketID uint32, socketFilename string, err error) {
	c := memif.NewServiceClient(conn)
	MemifSocketFilenameAddDel := memif.MemifSocketFilenameAddDel{
		IsAdd:          true,
		SocketID:       1,
		SocketFilename: "/run/vpp/memif.sock",
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
	mode := memif.MEMIF_MODE_API_ETHERNET
	memifCreate := &memif.MemifCreate{
		Role:     role,
		Mode:     mode,
		RxQueues: 1,
		TxQueues: 1,
		SocketID: socketID,
		RingSize: 1024,
		BufferSize: 2048,
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
	_, SwInterfaceSetFlagsErr := interfaces.NewServiceClient(conn).SwInterfaceSetFlags(ctx, swinterface)
	if err != nil {
		log.Entry(ctx).Fatalln("Set AdminUp ERROR:", SwInterfaceSetFlagsErr)
	}
}

func dumpMemif(ctx context.Context, conn api.Connection) {
	c := memif.NewServiceClient(conn)

	memifDumpMsg, err := c.MemifDump(ctx, &memif.MemifDump{})
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifDump failed:", err)
	}
	log.Entry(ctx).Infof("Socket file dump")
	for {
		memifDetails, err := memifDumpMsg.Recv()
		if err != nil {
			break
		}
		log.Entry(ctx).Infof(
			"SwIfIndex: %v\n"+
				"HwAddr: %v\n"+
				"ID: %v\n"+
				"Role: %v\n"+
				"Mode: %v\n"+
				"ZeroCopy: %v\n"+
				"SocketID: %v\n"+
				"RingSize: %v\n"+
				"BufferSize: %v\n"+
				"Flags: %v\n"+
				"IfName: %v\n",
			memifDetails.SwIfIndex, memifDetails.HwAddr, memifDetails.ID, memifDetails.Role, memifDetails.Mode, memifDetails.ZeroCopy,
			memifDetails.SocketID, memifDetails.RingSize, memifDetails.BufferSize, memifDetails.Flags, memifDetails.IfName)
	}
	log.Entry(ctx).Infof("Finish dumping from memif")
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

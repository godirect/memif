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
	"time"

	"git.fd.io/govpp.git/api"
	"github.com/edwarnicke/govpp/binapi/memif"
	"github.com/edwarnicke/log"
	"github.com/edwarnicke/vpphelper"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	// Connect to VPP with a 1 second timeout
	connectCtx, _ := context.WithTimeout(ctx, time.Second)
	conn, vppErrCh := vpphelper.StartAndDialContext(connectCtx)
	exitOnErrCh(ctx, cancel, vppErrCh)

	// Create a RPC client for the memif api
	isClient := true
	// Add a memif socket
	socketID, _ := createMemifSocket(ctx, conn)
	// Create a memif interface
	mode := memif.MEMIF_MODE_API_IP
	createMemif(ctx, conn, socketID, mode, isClient)
	// Delete a memif socket
	dumpMemif(ctx, conn)

	cancel()
	<-vppErrCh
}

func createMemifSocket(ctx context.Context, conn api.Connection) (socketID uint32, err error) {
	c := memif.NewServiceClient(conn)
	MemifSocketFilenameAddDel := memif.MemifSocketFilenameAddDel{
		IsAdd:          true,
		SocketID:       1,
		SocketFilename: "FirstSocketFile",
	}
	_, memifAddDel_err := c.MemifSocketFilenameAddDel(ctx, &MemifSocketFilenameAddDel)
	if memifAddDel_err != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifSocketFilenameAddDel failed:", memifAddDel_err)
	}

	log.Entry(ctx).Infof("Socket file created")
	log.Entry(ctx).Infof("SocketID: %v\n"+
		"SocketFilename: %v\n"+
		"IsAdd:%v\n",
		MemifSocketFilenameAddDel.SocketID, MemifSocketFilenameAddDel.SocketFilename, MemifSocketFilenameAddDel.IsAdd)
	return socketID, memifAddDel_err
}

func createMemif(ctx context.Context, conn api.Connection, socketID uint32, mode memif.MemifMode, isClient bool) error {
	role := memif.MEMIF_ROLE_API_MASTER
	if isClient {
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
	log.Entry(ctx).Infof("Memif interface created")
	log.Entry(ctx).Infof("swIfIndex"+
		"Role"+
		"SocketID",
		rsp.SwIfIndex, memifCreate.Role, memifCreate.SocketID)
	// ifindex.Store(ctx, isClient, rsp.SwIfIndex)
	return nil
}

func dumpMemif(ctx context.Context, conn api.Connection) (memif.RPCService_MemifDumpClient, error) {
	c := memif.NewServiceClient(conn)

	MemifDumpClient, MemifDumpErr := c.MemifDump(ctx, &memif.MemifDump{})
	if MemifDumpErr != nil {
		log.Entry(ctx).Fatalln("ERROR: MemifDump failed:", MemifDumpErr)
	}
	log.Entry(ctx).Infof("Socket file dump")
	return MemifDumpClient, MemifDumpErr
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

package main

import (
	"fmt"
	"testing"

	"context"
	"time"

	"git.fd.io/govpp.git/api"
	"git.fd.io/govpp.git/binapi/vpe"
	interfaces "github.com/edwarnicke/govpp/binapi/interface"
	"github.com/edwarnicke/govpp/binapi/interface_types"
	"github.com/edwarnicke/govpp/binapi/memif"
	"github.com/edwarnicke/log"
	"github.com/edwarnicke/vpphelper"
)
func TestSetupVppMaster(t *testing.T) {
	SetupVppMaster()
}

func SetupVppMaster() {
	ctx, cancel1 := context.WithCancel(context.Background())
	// Connect to VPP with a 1 second timeout
	connectCtx, _ := context.WithTimeout(ctx, time.Second)
	conn, vppErrCh := vpphelper.StartAndDialContext(connectCtx)
	exitOnErrCh(ctx, cancel1, vppErrCh)

	// Create a RPC client for the memif api
	isClient := true
	// Add a memif socket
	socketID1, _, _ := createMemifSocket(ctx, conn, "/run/vpp/memif1.sock", 1)
	// socketID2, _, _ := createMemifSocket(ctx, conn, "/run/vpp/memif1.sock", 2)
	// Create a memif interface
	createMemifInterface(ctx, conn, socketID1, isClient, memif.MEMIF_MODE_API_IP)
}


func createMemifSocket(ctx context.Context, conn api.Connection, socketFilename string, SocketID uint32) (uint32, string, error) {
	c := memif.NewServiceClient(conn)
	MemifSocketFilenameAddDel := memif.MemifSocketFilenameAddDel{
		IsAdd:          true,
		SocketID:       SocketID,
		SocketFilename: socketFilename,
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

func createMemifInterface(ctx context.Context, conn api.Connection, socketID uint32, isClient bool, mode memif.MemifMode) {
	role := memif.MEMIF_ROLE_API_MASTER
	if !isClient {
		role = memif.MEMIF_ROLE_API_SLAVE
	}
	// mode := memif.MEMIF_MODE_API_IP
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


	
	pingCmd := &vpe.CliInband{
		Cmd: fmt.Sprintf("set int state memif1/0 up"),
	}

	// Prime the pump, vpp doesn't arp until needed, and so the first ping will fail
	pingRsp, err := vpe.NewServiceClient(conn).CliInband(ctx, pingCmd)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: memifCreate failed:", err)
		return 
	}
	log.Entry(ctx).Infof(pingRsp.Reply)
	
	pingCmd = &vpe.CliInband{
		Cmd: fmt.Sprintf("set interface ip address memif1/0 10.0.0.2/24"),
	}

	// Prime the pump, vpp doesn't arp until needed, and so the first ping will fail
	pingRsp, err = vpe.NewServiceClient(conn).CliInband(ctx, pingCmd)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: memifCreate failed:", err)
		return 
	}
	log.Entry(ctx).Infof(pingRsp.Reply)

	pingCmd = &vpe.CliInband{
		Cmd: fmt.Sprintf("trace add memif-input 100"),
	}

	// Prime the pump, vpp doesn't arp until needed, and so the first ping will fail
	pingRsp, err = vpe.NewServiceClient(conn).CliInband(ctx, pingCmd)
	if err != nil {
		log.Entry(ctx).Fatalln("ERROR: memifCreate failed:", err)
		return 
	}
	log.Entry(ctx).Infof(pingRsp.Reply)
}


func vppConfig() {
	// setup ip addr
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
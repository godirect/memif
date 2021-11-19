/*
 *------------------------------------------------------------------
 * Copyright (c) 2020 Cisco and/or its affiliates.
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *------------------------------------------------------------------
 */

 package main

 import (
	 "bufio"
	 "flag"
	 "fmt"
	 "os"
	 "strings"
	 "sync"
	 "github.com/google/netstack/tcpip/header"
	 "github.com/pkg/profile"
	 "memif"
	//  "time"

	 "syscall"
 )
 const (
	EPOLLET        = 1 << 31
	MaxEpollEvents = 32
)
 func Disconnected(i *memif.Interface) error {
	 fmt.Println("Disconnected: ", i.GetName())
 
	 data, ok := i.GetPrivateData().(*interfaceData)
	 if !ok {
		 return fmt.Errorf("Invalid private data")
	 }
	 close(data.quitChan) // stop polling
	 close(data.errChan)
	 data.wg.Wait() // wait until polling stops, then continue disconnect
 
	 return nil
 }
 
 func Connected(i *memif.Interface) error {
	 fmt.Println("Connected: ", i.GetName())
 
	 data, ok := i.GetPrivateData().(*interfaceData)
	 if !ok {
		 return fmt.Errorf("Invalid private data")
	 }
	 data.errChan = make(chan error, 1)
	 data.quitChan = make(chan struct{}, 1)
	 data.wg.Add(1)
 
	 go func(errChan chan<- error, quitChan <-chan struct{}, wg *sync.WaitGroup) {
		 defer wg.Done()
		 echoPayload := getIcmpPacket()
		 fmt.Printf("echoPayload: %v\n",echoPayload)
		 // allocate packet buffer
		 pkt := make([]byte, 2048)
		 // get rx queue
		 rxq0, err := i.GetRxQueue(0)
		 if err != nil {
			 errChan <- err
			 return
		 }
		 // get tx queue
		 txq0, err := i.GetTxQueue(0)
		 if err != nil {
			 errChan <- err
			 return
		 }	
		 rxqFd, _:=rxq0.GetEventFd()
		 fmt.Printf("rx queue fd: %v\n", rxqFd)
		 buffer := make([]byte, 100)
		 syscall.Read(rxqFd, buffer)
		 ch := make(chan struct{}) // empty chan struct
		 go func() {
			handleEpoll(rxqFd, rxq0, pkt)
			close(ch)
		 } () 
		 txq0.WritePacket(echoPayload)
		 fmt.Println("Packet sent")
		 <-ch
	 }(data.errChan, data.quitChan, &data.wg)
 
	 return nil
 }
 
 type interfaceData struct {
	 errChan  chan error
	 quitChan chan struct{}
	 wg       sync.WaitGroup
 }
 
 func interractiveHelp() {
	 fmt.Println("help - print this help")
	 fmt.Println("start - start connecting loop")
	 fmt.Println("show - print interface details")
	 fmt.Println("exit - exit the application")
 }
 
 func main() {
	 cpuprof := flag.String("cpuprof", "", "cpu profiling output file")
	 memprof := flag.String("memprof", "", "mem profiling output file")
	 role := flag.String("role", "slave", "interface role")
	 name := flag.String("name", "gomemif", "interface name")
	 socketName := flag.String("socket", "", "control socket filename")
 
	 flag.Parse()
 
	 if *cpuprof != "" {
		 defer profile.Start(profile.CPUProfile, profile.ProfilePath(*cpuprof)).Stop()
	 }
	 if *memprof != "" {
		 defer profile.Start(profile.MemProfile, profile.ProfilePath(*memprof)).Stop()
	 }
 
	 memifErrChan := make(chan error)
	 exitChan := make(chan struct{})
 
	 var isMaster bool
	 switch *role {
	 case "slave":
		 isMaster = false
	 case "master":
		 isMaster = true
	 default:
		 fmt.Println("Invalid role")
		 return
	 }
 
	 fmt.Println("GoMemif: Responder")
	 fmt.Println("-----------------------")
 
	 socket, err := memif.NewSocket("gomemif_example", *socketName)
	 if err != nil {
		 fmt.Println("Failed to create socket: ", err)
		 return
	 }
 
	 data := interfaceData{}
	 args := &memif.Arguments{
		 IsMaster:         isMaster,
		 ConnectedFunc:    Connected,
		 DisconnectedFunc: Disconnected,
		 PrivateData:      &data,
		 Name:             *name,
	 }
 
	 i, err := socket.NewInterface(args)
	 if err != nil {
		 fmt.Println("Failed to create interface on socket %s: %s", socket.GetFilename(), err)
		 goto exit
	 }
 
	 // slave attempts to connect to control socket
	 // to handle control communication call socket.StartPolling()
	 if !i.IsMaster() {
		 fmt.Println(args.Name, ": Connecting to control socket...")
		 for !i.IsConnecting() {
			 err = i.RequestConnection()
			 if err != nil {
				 /* TODO: check for ECONNREFUSED errno
				  * if error is ECONNREFUSED it may simply mean that master
				  * interface is not up yet, use i.RequestConnection()
				  */
				 fmt.Println("Faild to connect: ", err)
				 goto exit
			 }
		 }
	 }
 
	 go func(exitChan chan<- struct{}) {
		 reader := bufio.NewReader(os.Stdin)
		 for {
			 fmt.Print("gomemif# ")
			 text, _ := reader.ReadString('\n')
			 // convert CRLF to LF
			 text = strings.Replace(text, "\n", "", -1)
			 switch text {
			 case "help":
				 interractiveHelp()
			 case "start":
				 // start polling for events on this socket
				 socket.StartPolling(memifErrChan)
			 case "show":
				 fmt.Println("remote: ", i.GetRemoteName())
				 fmt.Println("peer: ", i.GetPeerName())
			 case "exit":
				 err = socket.StopPolling()
				 if err != nil {
					 fmt.Println("Failed to stop polling: ", err)
				 }
				 close(exitChan)
				 return
			 default:
				 fmt.Println("Unknown input")
			 }
		 }
	 }(exitChan)
 
	 for {
		 select {
		 case <-exitChan:
			 goto exit
		 case err, ok := <-memifErrChan:
			 if ok {
				 fmt.Println(err)
			 }
		 case err, ok := <-data.errChan:
			 if ok {
				 fmt.Println(err)
			 }
		 default:
			 continue
		 }
	 }
 
 exit:
	 socket.Delete()
	 close(memifErrChan)
 }

 func handleEpoll(fd int, qid *memif.Queue, pkt []byte) {
	var event syscall.EpollEvent
	var events [MaxEpollEvents]syscall.EpollEvent

	defer syscall.Close(fd)

	err := syscall.SetNonblock(fd, true);
	if err != nil {
		fmt.Println("setnonblock1: ", err)
		os.Exit(1)
	}

	epfd, e := syscall.EpollCreate1(0)
	if e != nil {
		fmt.Println("epoll_create1: ", e)
		os.Exit(1)
	}
	defer syscall.Close(epfd)

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fd)
	if e = syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, fd, &event); e != nil {
		fmt.Println("epoll_ctl: ", e)
		os.Exit(1)
	}
	
	for {
		nevents, e := syscall.EpollWait(epfd, events[:], 10)
		if e != nil {
			fmt.Println("epoll_wait: ", e)
			break
		}
		// fmt.Printf("Get event %v\n", nevents)
		for ev := 0; ev < nevents; ev++ {
			if int(events[ev].Fd) == fd { // handle weired
				fmt.Println("Found")

				_, Rxerr := qid.ReadPacket(pkt)
				fmt.Print("Start printing")
				fmt.Printf("%v\n",string(pkt))
				if Rxerr != nil {
					fmt.Print("Error")
					// errChan <- Rxerr
					return
				}		
			} else {
				// Error handling 
				fmt.Println("Should not reach here")	
			}
		}

	}

 }

 func getIcmpPacket() header.IPv4 {
	ipv4Header := header.IPv4{0x45, 0x00, 0x00, 0x1C, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 0x0A, 0x00, 0x00, 0x01, 0x0A, 0x00, 0x00, 0x02, 0x08, 0x00, 0xf7, 0xff, 0x00, 0x00, 0x00, 0x00 }
	ipv4Checksum := ipv4Header.CalculateChecksum()
	ipv4Header.EncodePartial(ipv4Checksum, 28)
	// icmpHeader := header.ICMPv4{0x08, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00}
	// fmt.Print(ipv4Header)
	return ipv4Header
}
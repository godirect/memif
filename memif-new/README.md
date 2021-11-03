Reproduction instructions for #Ping# Example:
1. Environment Build
1.1 Go under memif-new/ and run `docker build`
1.2 Run `docker run -it -d -v "$(pwd)"/:/workdir [docker image id] /bin/bash` in your teminal 
1.3 Run `docker exec -it [container id] /bin/bash` in another terminal

2. Start a memif interface and configurations
2.1Inside one of the terminali, go under _example/vpp and run `./vpp` to start a process and create a memif interface
2.2 run `vppctl`
2.3 run `set int state memif1/0 up`
2.4 run `set interface ip address memif1/0 10.0.0.2/24`

3. Run ping process
3.1 In another teminal, go under _example/ping and run `./ping --socket /run/vpp/memif1.sock --name memif1/0`
3.2 run `start`

After the above steps, the ping process will send a ping message to the VPP master and the VPP master should return the ping reply message back to the ping process:)
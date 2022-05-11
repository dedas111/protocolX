# Anonymous messaging using mix networks

## Setup

To build and test the code you need:

* Go 1.9 or later

To perform the unit tests run:

```shell
go test ./...
```

Before first fresh run of the system run:

```shell
bash clean.sh
```

This removes all log files and database

## Usage

To run the network, i.e., mixnodes and providers run

```shell
bash run_network.sh
```

This spins up 3 mixnodes and 1 provider

To simulate the clients run

```shell
bash run_clients.sh
```

## Debugging

To run a single node statically as compute without changing the role run
```shell
go run main.go -typ=provider -id="1" -port=9900 -staticRole=compute -computeListenerCount=8 -funnelListenerCount=4
```
This expects compute nodes to have 8 listener ports and funnels to have 4 listener ports.
IDs should be assigned to funnels first as the current connection establishment to them uses the modulo operation.

Same for funnels
```shell
go run main.go -typ=provider -id="0" -port=9900 -staticRole=funnel -computeListenerCount=8 -funnelListenerCount=4
```

## Server Testing
Server tests currently require the developer to start local instances of compute and funnel nodes.
This is preferably done by setting their roles static using the above commands.\
Furthermore one needs to change **remoteIP**, **localIP** and **listOfComputeIPs** to the local IP (note: Golang uses the default interface and not the loopback interface) of the machine in **server_test.go**.\
**privP**, **pubP** and certificates are also required. Make sure that hardcoded paths are correct (e.g. by using search and replace).\
Start with giving IDs to the funnels starting with 0. After all funnels have an ID, continue with the compute nodes.\
There is no order for starting nodes.\
Watch out for the amount of funnels (**numOfFunnels** in **server.go**) if in a case with static roles.\
The parameter **computeListeners** is overridden by cli parameters and can be used to tell funnel nodes how many ports a compute has. Can be omitted for compute nodes.\
For the usage of funnel nodes, **idForFunnelConn** and **numOfFunnelPortsToUse** were introduced. Each compute gets a unique ID starting with 0 and the number of funnel ports to use. Compute will connect to the funnel without any funnel port used multiple times. A total of **numOfFunnelPortsToUse** times the amount of compute nodes will be used on the funnel. They are expected to be available. 
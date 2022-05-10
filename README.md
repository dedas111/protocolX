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
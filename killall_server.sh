#!/bin/sh

kill_port() {
    PID=$(lsof -t -i:$1)
    echo $PID
    #kill -TERM "$PID" || kill -KILL "$PID"
}

echo "** Killing all leftover servers in portrange 9000 - 9999 **"
for (( j=0; j<100; j++ ));
do
  #echo "$((9900+$j))"
  kill_port $((9900+$j))
done
echo "** Done killing **"
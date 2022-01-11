#!/bin/sh
# run before multiple funnels test

echo "Press CTRL-C to stop."
rm -rf "$(pwd)/pki/database.db"

logDir="$(pwd)/logs"

if [ -d $logDir ]
then
    echo "Logging directory already exists"
else
    mkdir $logDir
    echo "Created logging directory"
fi

NUMMIXES=3
for (( j=0; j<$NUMMIXES; j++ ));
do
	#echo $((9900+j*10))
	go run main.go -typ=provider -id="$j" -host=localhost -port=$((9900+j*10)) >> logs/bash.log &
	sleep 1
done

# read -p "Press CTRL-C to stop."

# In case the loop is not working, we can use the following command
#go run main.go -typ=mix -id=Mix1 -host=localhost -port=9998 > logs/bash.log &


# trap call ctrl_c()
trap ctrl_c SIGINT SIGTERM SIGTSTP
function ctrl_c() {
        echo "** Trapped SIGINT, SIGTERM and SIGTSTP"
        for (( j=0; j<$NUMMIXES; j+20 ));
        do
            kill_port $((9980+$j))
        done
}

function kill_port() {
    PID=$(lsof -t -i:$1)
    echo "$PID"
    kill -TERM $PID || kill -KILL $PID
}





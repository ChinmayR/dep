#!/bin/bash

while [[ "$#" > 1 ]]; do case $1 in
    --host) host="$2";;
    --port) port="$2";;
    --name) name="$2";;
    --interval) interval="$2";;
    --tries) tries="$2";;
    --cqlversion) cqlversion="$2";;
    *) break;;
  esac; shift; shift
done

if [[ -z $host || ! $port =~ ^[0-9]+$ ]]
then
    echo "ERROR:: Need --host and --port"
    exit 1
fi

: ${name:=$host:$port}
: ${interval:=1}
: ${tries:=10}

for i in $(seq 1 $tries)
do
    if ! cqlsh --cqlversion "$cqlversion" "$host" "$port" -e "SHOW HOST" 2>/dev/null
    then
        echo "waiting for $name"
        sleep $interval
    else
        echo "$name is ready!"
        exit 0
    fi
    if (( $i == $tries ))
    then
        echo "ERROR:: Timeout reached while waiting for $name"
        exit 1
    fi
done

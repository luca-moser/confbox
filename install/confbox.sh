#!/bin/bash

if [[ $1 == 'start' ]]
then
        echo 'starting confbox...'
        docker-compose -p confbox up -d
elif [[ $1 == 'stop' ]]
then
        echo 'stoppping confbox...'
        docker-compose -p confbox stop
elif [[ $1 == 'restart' ]]
then
        echo 'restarting confbox...'
        docker-compose -p confbox restart
elif [[ $1 == 'reinit' ]]
then
        echo 'reinitialising confbox...'
        docker-compose -p confbox stop
        docker-compose -p confbox rm -f
        docker-compose -p confbox up -d
elif [[ $1 == 'destroy' ]]
then
        echo 'destroying confbox containers...'
        docker-compose -p confbox rm -f
else
        echo 'commands: <start,stop,restart,reinit,destroy>'
fi
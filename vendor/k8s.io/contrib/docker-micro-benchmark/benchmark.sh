#!/bin/bash

# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

RESULT=result
DOCKER_MICRO_BENCHMARK=./docker-micro-benchmark
STAT_TOOL=pidstat
GNUPLOT=gnuplot
PLOTDIR=plot
AWK=awk

usage () {
  echo "Usage : `basename $0` -[o|c|i|r]"
  exit
}

# $1 parameter, $2 benchmark name
doBenchmark() {
  RDIR=$RESULT/$2
  if [ ! -d  $RDIR ]; then
    mkdir $RDIR
  fi
  LC_ALL=C sar -rubwS -P ALL 1 > $RDIR/sar_benchmark.dat &
  SAR_PID=$!
  $DOCKER_MICRO_BENCHMARK $1 > $RDIR/result_benchmark.dat &
  BENCHMARK_PID=$!
  $STAT_TOOL -p $BENCHMARK_PID 1 > $RDIR/cpu_benchmark.dat &
  DOCKER_PID=`pidof dockerd`
  if [ $? -ne 0 ]; then
    # Be compatible with old version.
    DOCKER_PID=`pidof docker`
  fi
  $STAT_TOOL -p $DOCKER_PID 1 > $RDIR/cpu_docker_daemon.dat &
  DOCKER_PIDSTAT=$!
  # For old version without containerd, there is no data for containerd,
  # and the corresponding data won't be plotted.
  CONTAINERD_PID=`pidof docker-containerd`
  $STAT_TOOL -p $CONTAINERD_PID 1 > $RDIR/cpu_containerd_daemon.dat &
  CONTAINERD_PIDSTAT=$!
  wait $BENCHMARK_PID
  kill $CONTAINERD_PIDSTAT
  kill $DOCKER_PIDSTAT
  kill $SAR_PID
  doParse $2
}

# $1 benchmark name
doParse() {
  RDIR=$RESULT/$1
  DATA=result_benchmark.dat
  TMP=tmp
  TYPE=png
  cd $RDIR
  if [ -d $TMP ]; then
    rm -r $TMP
  fi
  mkdir $TMP
  $AWK '/^$/{getline file; "'${TMP}'/"file < /dev/null ; next} !/^$/{print >> "'${TMP}'/"file}' < $DATA
  for file in `ls $TMP`; do
    $GNUPLOT -e "ifilename='${TMP}/$file'; ofilename='latency-$file.$TYPE'" ../../$PLOTDIR/latency_plot
    $GNUPLOT -e "ifilename='${TMP}/$file'; ofilename='$file.$TYPE'" ../../$PLOTDIR/$1/result_plot
  done
  $GNUPLOT ../../$PLOTDIR/cpu_plot
  rm -r $TMP
  cd - > /dev/null
}

if [ -z $1 ]; then
  usage
  exit 1
fi

if [ ! -d $RESULT ]; then
  mkdir $RESULT
fi

while [ "$1" != "" ]; do
  case $1 in
    -o )
      echo "Benchmark container operations"
      doBenchmark $1 container_op
      shift
      ;;
    -c )
      CONTAINER_NUMBER=`docker ps -a | wc -l`
      CONTAINER_NUMBER=`expr $CONTAINER_NUMBER - 1`
      if [ $CONTAINER_NUMBER -ne 0 ]; then
        shell/remove_all_containers.sh > /dev/null
      fi 
      echo "Benchmark with different container numbers"
      doBenchmark $1 varies_containers
      shift
      ;;
    -i )
      echo "Benchmark with different intervals"
      doBenchmark $1 varies_intervals
      shift
      ;;
    -r )
      echo "Benchmark with different goroutine numbers"
      doBenchmark $1 varies_routines
      shift
      ;;
    * )
      usage
      exit 1
      ;;
  esac
done

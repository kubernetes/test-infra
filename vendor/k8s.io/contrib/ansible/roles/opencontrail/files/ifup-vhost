#!/bin/bash

. /etc/init.d/functions

cd /etc/sysconfig/network-scripts
. ./network-functions

[ -f ../network ] && . ../network

CONFIG=${1}

need_config "${CONFIG}"

source_config

if ! /sbin/modprobe vrouter >/dev/null 2>&1; then
   net_log $"OpenContrail vrouter kernel module not available"
   exit 1
fi

if [ -n "${MACADDR}" ]; then
    hwaddr=${MACADDR}
else
    if [ -n "${PHYSDEV}" ]; then
	hwaddr=$(cat /sys/class/net/${PHYSDEV}/address)
    fi
fi

if [ ! -d /sys/class/net/${DEVICE} ]; then
    ip link add ${DEVICE} type vhost

    if [ -n "${hwaddr}" ]; then
	ip link set ${DEVICE} address ${hwaddr}
    fi

    if [ -n "${PHYSDEV}" ]; then
	vif --add ${PHYSDEV} --mac ${hwaddr} --vrf 0 --vhost-phys --type physical >/dev/null 2>&1
	vif --add ${DEVICE} --mac ${hwaddr} --vrf 0 --type vhost --xconnect ${PHYSDEV} >/dev/null 2>&1
    fi
fi

if [ -n "${IPADDR}" ]; then
    ip addr add dev ${DEVICE} ${IPADDR}
fi

ip link set ${DEVICE} up

exec /etc/sysconfig/network-scripts/ifup-post ${CONFIG} ${2}


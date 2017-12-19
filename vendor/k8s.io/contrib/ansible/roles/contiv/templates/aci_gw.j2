#!/bin/bash

usage="$0 start"
if [ $# -ne 1 ]; then
    echo USAGE: $usage
    exit 1
fi

case $1 in
start)
    set -e

    docker run --net=host \
    -e "APIC_URL={{ apic_url }}" \
    -e "APIC_USERNAME={{ apic_username }}" \
    -e "APIC_PASSWORD={{ apic_password }}" \
    -e "APIC_LEAF_NODE={{ apic_leaf_nodes }}" \
    -e "APIC_PHYS_DOMAIN={{ apic_phys_dom }}" \
    -e "APIC_EPG_BRIDGE_DOMAIN={{ apic_epg_bridge_domain }}" \
    -e "APIC_CONTRACTS_UNRESTRICTED_MODE={{ apic_contracts_unrestricted_mode }}" \
    --name=contiv-aci-gw \
    contiv/aci-gw
    ;;

stop)
    # don't stop on error
    docker stop contiv-aci-gw
    docker rm contiv-aci-gw
    ;;

*)
    echo USAGE: $usage
    exit 1
    ;;
esac

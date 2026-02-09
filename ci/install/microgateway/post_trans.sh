#!/bin/sh

if command -V systemctl >/dev/null 2>&1; then
    if [ ! -f /lib/systemd/system/tyk-microgateway.service ]; then
        cp /opt/tyk-microgateway/install/inits/systemd/system/tyk-microgateway.service /lib/systemd/system/tyk-microgateway.service
    fi
else
    if [ ! -f /etc/init.d/tyk-microgateway ]; then
        cp /opt/tyk-microgateway/install/inits/sysv/init.d/tyk-microgateway /etc/init.d/tyk-microgateway
    fi
fi

#!/bin/bash
# EC2 user data for every benchmark VM (Ubuntu 24.04). aws.sh passes it to
# run-instances; it holds no secrets, because user data is readable by anyone
# who can describe the instance. Secrets are copied over SSH by `aws.sh deploy`.
#
# It installs Docker and chrony and applies the kernel settings from
# RUNBOOK-cloud.md, persistently, so they survive a reboot.
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive

apt-get update -q
apt-get install -y -q docker.io docker-compose-v2 chrony jq curl iputils-ping rsync
systemctl enable --now docker chrony
usermod -aG docker ubuntu

cat > /etc/sysctl.d/90-gwbench.conf <<'EOF'
net.core.somaxconn = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_tw_reuse = 1
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_max_syn_backlog = 65535
fs.file-max = 2097152
EOF
sysctl --system

cat > /etc/security/limits.d/90-gwbench.conf <<'EOF'
*    soft nofile 1048576
*    hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
EOF

# Distroless images run as uid 65532 with a root-owned WORKDIR, so anything
# they write goes to a bind mount owned by that uid.
install -d -m 755 /opt/gwbench
install -d -m 700 -o 65532 -g 65532 /opt/gwbench/data
chown -R ubuntu:ubuntu /opt/gwbench
chown 65532:65532 /opt/gwbench/data

touch /var/lib/gwbench-init-done

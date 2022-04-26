#!/bin/sh

mkdir -p /tmp/freegeoip

[ -f "/db.gz" ] && cp /db.gz /tmp/freegeoip/db.gz

exec /go/bin/freegeoip
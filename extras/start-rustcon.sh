#!/bin/bash
# 

SERVER=$1

. /etc/rustcon/$SERVER.vars

echo "Starting up rustcon for $SERVER on $HOST:$PORT."

/opt/rustcon/rustcon \
	-config /opt/rustcon/rustcon.conf \
	-hostname $HOST \
	-port $PORT \
	-tag $TAG \
	-passfile $PASSFILE
